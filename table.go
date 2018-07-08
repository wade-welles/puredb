package puredb

import (
	"reflect"
	"fmt"
	"log"
	"github.com/dgraph-io/badger"
	"os"
	"github.com/vmihailenco/msgpack"
	"time"
)

type Table struct {
	DB *PureDB
	badgerDB *badger.DB

	Name string

	buckets *buckets

	structInfo	*structInfo
}

type structInfo struct {
	pTyp	reflect.Type
	typ		reflect.Type

	fields	[]*reflect.StructField
	fieldByName map[string]*reflect.StructField

	indexes	[]*indexInfo
	primary	*indexInfo
	indexByName map[string]*indexInfo
}

type indexInfo struct {
	name		string
	primary		bool
	unique		bool
	field		*reflect.StructField
	bucketOpts	BucketOpts
	bucket		*Bucket
}

func (table *Table) Badger() *badger.DB {
	return table.badgerDB
}

func (table *Table) Setup(db *PureDB, name string) error {
	table.DB = db
	table.badgerDB = db.DB
	table.Name = name

	table.buckets = &buckets{}
	table.buckets.Init(table.DB)

	return nil
}

func (table *Table) Cleanup() {
	table.buckets.Cleanup()
}

func (table *Table) GetName() string {
	return table.Name
}

func (table *Table) GetOrCreateBucket(name string, opts BucketOpts) *Bucket {
	finalBucketName := table.Name + "." + name
	if table.buckets.Has(finalBucketName) {
		return table.buckets.Get(finalBucketName)
	}
	bucket, err := table.buckets.Add(finalBucketName, opts)
	if err != nil {
		panic(err)
	}
	return bucket
}

func (table *Table) Save(v interface{}) (int64, error) {
	log.Printf("PureDB.Save(%v)", v)

	sInfo, err := table.getStructInfo(v)
	if err != nil {
		return -1, err
	}
	//pTyp := reflect.TypeOf(v)
	//pVal := reflect.ValueOf(v)
	//log.Printf("[1] typ:%v kind:%v val:%v", pTyp, pTyp.Kind(), pVal)
	//if pTyp.Kind() != reflect.Ptr {
	//	return -1, fmt.Errorf("not a pointer")
	//}
	//typ := pTyp.Elem()
	//val := pVal.Elem()
	//log.Printf("[2] typ:%v kind:%v val:%v", typ, typ.Kind(), val)
	//if typ.Kind() != reflect.Struct {
	//	return -1, fmt.Errorf("not a pointer to a struct")
	//}
	//
	//////val := reflect.ValueOf(v)
	////s := val
	////typ = s.Type()
	////log.Printf("typ:%v kind:%v", typ, typ.Kind())
	////if typ.Kind() != reflect.Struct {
	////	return -1, fmt.Errorf("not a pointer to a struct")
	////}

	pVal := reflect.ValueOf(v)
	val := pVal.Elem()

	primaryId := int64(-1)
	//var err error

	primaryId, err = sInfo.primary.bucket.Add(v)
	fmt.Fprintf(os.Stderr, "adding record to primary bucket v:%v id:%v err:%v\n", v, primaryId, err)
	if err != nil {
		return -1, err
	}

	for _, index := range sInfo.indexes {
		if index.primary {
			continue
		}
		fieldVal := val.FieldByIndex(index.field.Index)
		//fieldVal := val.FieldByName(index.field.Name)
		fmt.Fprintf(os.Stderr, "secondary index %v val:%v index:%v sf:%v\n", index.name, fieldVal, index, index.field)
		err = index.bucket.Set(fieldVal.Interface(), primaryId)
		if err != nil {
			return -1, err
		}
		fmt.Fprintf(os.Stderr, "adding record to index %v bucket id:%v err:%v\n", index.name, primaryId, err)
	}

	//for i := 0; i < sInfo.typ.NumField(); i++ {
	//	fieldVal := val.Field(i)
	//	structField := sInfo.typ.Field(i)
	//
	//	tag, _ := structField.Tag.Lookup("puredb")
	//	tagOpts := parseTag(tag)
	//
	//	fmt.Printf("[%d] fv:%30v sf:%v - %s %s = %v tag:%v\n",
	//		i, fieldVal, structField,
	//		structField.Name, fieldVal.Type(), fieldVal.Interface(), tagOpts)
	//
	//	fieldName := structField.Name
	//
	//	if tagOpts.Has("primary") {
	//		primaryBucketOpts := BucketOpts{}
	//		primaryBucketOpts = BucketOptsIntInt
	//		primaryBucketOpts.PreAddFn = func (bucket *Bucket, k interface{}, v interface{}) error {
	//			id := k.(int64)
	//			// reflect.ValueOf(v).Elem().Type().AssignableTo(typ) <=> v.(*myStruct)
	//			if ! reflect.ValueOf(v).Elem().Type().AssignableTo(typ) {
	//				return fmt.Errorf("value doesn't implement pointer to (specified) struct interface")
	//			}
	//
	//			//typ := reflect.TypeOf(v).Elem()
	//			val := reflect.ValueOf(v).Elem()
	//			idFieldVal := val.FieldByName(fieldName)
	//			if idFieldVal.CanSet() {
	//				fmt.Fprintf(os.Stderr, "PreAddFn set id:%v OK\n", id)
	//				idFieldVal.SetInt(id)
	//			} else {
	//				return fmt.Errorf("id field can't be set")
	//			}
	//			return nil
	//		}
	//		primaryBucketOpts.MarshalValueFn = func (v interface{}) ([]byte, error) {
	//			fmt.Fprintf(os.Stderr, "Marshaling struct v:%v...\n", v)
	//			if ! reflect.ValueOf(v).Elem().Type().AssignableTo(typ) {
	//				return []byte{}, fmt.Errorf("value doesn't implement pointer to (specified) struct interface")
	//			}
	//			pVal := reflect.ValueOf(v)
	//			// pVal.Interface() <=> v.(*myStruct)
	//			fmt.Fprintf(os.Stderr, "Marshalling struct v:%v from interface:%v\n", v, pVal.Interface())
	//			return msgpack.Marshal(pVal.Interface())
	//		}
	//		primaryBucketOpts.UnmarshalValueFn = func (data []byte, v *interface{}) error {
	//			fmt.Fprintf(os.Stderr, "Unmarshaling struct...\n")
	//			structPtr := reflect.New(typ)
	//			err := msgpack.Unmarshal(data, structPtr.Interface())
	//			// XXX TODO: perform post-hydration logic
	//			if err != nil {
	//				return err
	//			}
	//			*v = structPtr.Elem().Interface()
	//			fmt.Fprintf(os.Stderr, "Unmarshaled struct v:%v\n", *v)
	//			return nil
	//		}
	//		fmt.Fprintf(os.Stderr, "going to add record to primary bucket v:%v...\n", v)
	//		bucket := table.GetOrCreateBucket(structField.Name, primaryBucketOpts)
	//		primaryId, err = bucket.Add(v)
	//		fmt.Fprintf(os.Stderr, "adding record to primary bucket v:%v id:%v err:%v\n", v, primaryId, err)
	//		if err != nil {
	//			return -1, err
	//		}
	//	} else if tagOpts.Has("index") {
	//		indexBucketOpts := BucketOpts{}
	//		indexBucketOpts = BucketOptsIntInt
	//
	//		if fieldVal.Type() == reflect.TypeOf(time.Time{}) {
	//			indexBucketOpts.MarshalKeyFn = func (v interface{}) ([]byte, error) {
	//				t, ok := v.(time.Time)
	//				if ! ok {
	//					return nil, fmt.Errorf("not a valid time object: %v", v)
	//				}
	//				return t.MarshalBinary()
	//			}
	//			indexBucketOpts.UnmarshalKeyFn = func (data []byte, v *interface{}) error {
	//				t := time.Time{}.UTC()
	//				err := t.UnmarshalBinary(data)
	//				if err != nil {
	//					return err
	//				}
	//				*v = t
	//				return nil
	//			}
	//		}
	//		fmt.Fprintf(os.Stderr, "going to add record to index %v bucket id:%v (value %v)...\n", fieldName, primaryId, fieldVal.Interface())
	//		bucket := table.GetOrCreateBucket(structField.Name, indexBucketOpts)
	//		err = bucket.Set(fieldVal.Interface(), primaryId)
	//		fmt.Fprintf(os.Stderr, "adding record to index %v bucket id:%v err:%v\n", fieldName, primaryId, err)
	//	}
	//}

	return primaryId, nil
}

func (table *Table) getStructInfo(v interface{}) (*structInfo, error) {
	pTyp := reflect.TypeOf(v)
	pVal := reflect.ValueOf(v)
	log.Printf("[1] typ:%v kind:%v val:%v", pTyp, pTyp.Kind(), pVal)
	if pTyp.Kind() != reflect.Ptr {
		return nil, fmt.Errorf("not a pointer")
	}
	typ := pTyp.Elem()
	val := pVal.Elem()
	log.Printf("[2] typ:%v kind:%v val:%v", typ, typ.Kind(), val)
	if typ.Kind() != reflect.Struct {
		return nil, fmt.Errorf("not a pointer to a struct")
	}

	if table.structInfo != nil {
		if (table.structInfo.pTyp != pTyp) || (table.structInfo.typ != typ) {
			return nil, fmt.Errorf("struct doesn't match the blueprint one")
		}
		return table.structInfo, nil
	}

	sInfo := &structInfo{
		pTyp: pTyp,
		typ: typ,
		fields: make([]*reflect.StructField, 0),
		fieldByName: make(map[string]*reflect.StructField),
		indexes: make([]*indexInfo, 0),
		indexByName: make(map[string]*indexInfo),
	}
	table.structInfo = sInfo

	for i := 0; i < typ.NumField(); i++ {
		fieldVal := val.Field(i)
		structField := typ.Field(i)

		fieldName := structField.Name

		sInfo.fields = append(sInfo.fields, &structField)
		sInfo.fieldByName[fieldName] = &structField

		tag, _ := structField.Tag.Lookup("puredb")
		tagOpts := parseTag(tag)

		fmt.Printf("[%d] fv:%30v sf:%v - %s %s = %v tag:%v\n",
			i, fieldVal, structField,
			structField.Name, fieldVal.Type(), fieldVal.Interface(), tagOpts)

		if tagOpts.Has("primary") {
			primaryBucketOpts := BucketOpts{}
			primaryBucketOpts = BucketOptsIntInt
			primaryBucketOpts.PreAddFn = func (bucket *Bucket, k interface{}, v interface{}) error {
				id := k.(int64)
				// reflect.ValueOf(v).Elem().Type().AssignableTo(typ) <=> v.(*myStruct)
				if ! reflect.ValueOf(v).Elem().Type().AssignableTo(typ) {
					return fmt.Errorf("value doesn't implement pointer to (specified) struct interface")
				}

				//typ := reflect.TypeOf(v).Elem()
				val := reflect.ValueOf(v).Elem()
				idFieldVal := val.FieldByName(fieldName)
				if idFieldVal.CanSet() {
					fmt.Fprintf(os.Stderr, "PreAddFn set id:%v OK\n", id)
					idFieldVal.SetInt(id)
				} else {
					return fmt.Errorf("id field can't be set")
				}
				return nil
			}
			primaryBucketOpts.MarshalValueFn = func (v interface{}) ([]byte, error) {
				fmt.Fprintf(os.Stderr, "Marshaling struct v:%v...\n", v)
				if ! reflect.ValueOf(v).Elem().Type().AssignableTo(typ) {
					return []byte{}, fmt.Errorf("value doesn't implement pointer to (specified) struct interface")
				}
				pVal := reflect.ValueOf(v)
				// pVal.Interface() <=> v.(*myStruct)
				fmt.Fprintf(os.Stderr, "Marshalling struct v:%v from interface:%v\n", v, pVal.Interface())
				return msgpack.Marshal(pVal.Interface())
			}
			primaryBucketOpts.UnmarshalValueFn = func (data []byte, v *interface{}) error {
				fmt.Fprintf(os.Stderr, "Unmarshaling struct...\n")
				structPtr := reflect.New(typ)
				err := msgpack.Unmarshal(data, structPtr.Interface())
				// XXX TODO: perform post-hydration logic
				if err != nil {
					return err
				}
				*v = structPtr.Elem().Interface()
				fmt.Fprintf(os.Stderr, "Unmarshaled struct v:%v\n", *v)
				return nil
			}
			fmt.Fprintf(os.Stderr, "going to add record to primary bucket v:%v...\n", v)
			bucket := table.GetOrCreateBucket(structField.Name, primaryBucketOpts)
			primaryIndex := indexInfo{
				name:		fieldName,
				primary:	true,
				unique:		true,
				field:		&structField,
				bucketOpts:	primaryBucketOpts,
				bucket:		bucket,
			}
			sInfo.primary = &primaryIndex
			sInfo.indexes = append(sInfo.indexes, &primaryIndex)
			sInfo.indexByName[fieldName] = &primaryIndex
			fmt.Fprintf(os.Stderr, "setup primary index %v %v bucket:%v\n", fieldName, primaryIndex, bucket.Name)
		} else if tagOpts.Has("index") {
			indexBucketOpts := BucketOpts{}
			indexBucketOpts = BucketOptsIntInt

			fmt.Fprintf(os.Stderr, "setup secondary index %v type:%v\n", fieldName, fieldVal.Type())
			if fieldVal.Type() == reflect.TypeOf(time.Time{}) {
				fmt.Fprintf(os.Stderr, "secondary index %v is time.Time\n", fieldName)
				indexBucketOpts.MarshalKeyFn = func (v interface{}) ([]byte, error) {
					t, ok := v.(time.Time)
					if ! ok {
						return nil, fmt.Errorf("not a valid time object: %v", v)
					}
					return t.MarshalBinary()
				}
				indexBucketOpts.UnmarshalKeyFn = func (data []byte, v *interface{}) error {
					t := time.Time{}.UTC()
					err := t.UnmarshalBinary(data)
					if err != nil {
						return err
					}
					*v = t
					return nil
				}
			}
			bucket := table.GetOrCreateBucket(structField.Name, indexBucketOpts)
			secondaryIndex := indexInfo{
				name:		fieldName,
				primary:	false,
				unique:		false,
				field:		&structField,
				bucketOpts:	indexBucketOpts,
				bucket:		bucket,
			}
			sInfo.indexes = append(sInfo.indexes, &secondaryIndex)
			sInfo.indexByName[fieldName] = &secondaryIndex
			fmt.Fprintf(os.Stderr, "setup index %v %v bucket:%v\n", fieldName, secondaryIndex, bucket.Name)
		}
	}

	return sInfo, nil
}

func stringInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}


type tables struct {
	DB	*PureDB
	Map	map[string]*Table
}

func (tables *tables) Init(db *PureDB) {
	tables.DB = db
	tables.Map = make(map[string]*Table)
}

func (tables *tables) Cleanup() {
	for _, ti := range tables.Map {
		ti.Cleanup()
	}
}

func (tables *tables) Add(name string) (*Table, error) {
	log.Printf("tables::Add name:%v", name)
	table := Table{}
	err := table.Setup(tables.DB, name)
	if err != nil {
		return nil, err
	}
	tables.Map[name] = &table
	return &table, nil
}

func (tables *tables) Get(name string) *Table {
	return tables.Map[name]
}

func (tables *tables) Has(name string) bool {
	_, ok := tables.Map[name]
	return ok
}
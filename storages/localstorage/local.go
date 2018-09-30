package localstorage

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"fmt"
	"reflect"
	"strconv"

	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/iterators"
	"github.com/adamluzsi/frameless/queries/destroy"
	"github.com/adamluzsi/frameless/queries/find"
	"github.com/adamluzsi/frameless/queries/update"
	"github.com/adamluzsi/frameless/reflects"
	"github.com/boltdb/bolt"
)

func NewLocal(path string) (*Local, error) {
	db, err := bolt.Open(path, 0600, nil)

	return &Local{DB: db}, err
}

type Local struct {
	DB *bolt.DB
}

// Close the Local database and release the file lock
func (storage *Local) Close() error {
	return storage.DB.Close()
}

func (storage *Local) Store(e frameless.Entity) error {
	return storage.DB.Update(func(tx *bolt.Tx) error {

		bucket, err := storage.ensureBucketFor(tx, e)

		if err != nil {
			return err
		}

		uIntID, err := bucket.NextSequence()

		if err != nil {
			return err
		}

		encodedID := strconv.FormatUint(uIntID, 10)

		if err = reflects.SetID(e, encodedID); err != nil {
			return err
		}

		value, err := storage.encode(e)

		if err != nil {
			return err
		}

		return bucket.Put(storage.uintToBytes(uIntID), value)

	})
}

func (storage *Local) Exec(quc frameless.Query) frameless.Iterator {
	switch quc := quc.(type) {
	case find.ByID:
		key, err := storage.idToBytes(quc.ID)

		if err != nil {
			return iterators.NewError(err)
		}

		entity := reflect.New(reflect.TypeOf(quc.Type)).Interface()

		err = storage.DB.View(func(tx *bolt.Tx) error {
			bucket, err := storage.bucketFor(tx, quc.Type)

			if err != nil {
				return err
			}

			encodedValue := bucket.Get(key)

			if encodedValue == nil {
				entity = nil
				return nil
			}

			return storage.decode(encodedValue, entity)
		})

		if err != nil {
			return iterators.NewError(err)
		}

		if entity == nil {
			return iterators.NewEmpty()
		}

		return iterators.NewSingleElement(entity)

	case find.All:
		r, w := iterators.NewPipe()

		go storage.DB.View(func(tx *bolt.Tx) error {
			defer w.Close()

			bucket := tx.Bucket(storage.bucketName(quc.Type))

			if bucket == nil {
				return nil
			}

			if err := bucket.ForEach(func(IDbytes, encodedEntity []byte) error {
				entity := reflect.New(reflect.TypeOf(quc.Type)).Interface()
				storage.decode(encodedEntity, entity)
				return w.Send(entity) // iterators.ErrClosed will cancel ForEach execution
			}); err != nil {
				w.Error(err)
				return err
			}

			return nil
		})

		return r

	case destroy.ByID:

		ID, err := storage.idToBytes(quc.ID)

		if err != nil {
			return iterators.NewError(err)
		}

		return iterators.NewError(storage.DB.Update(func(tx *bolt.Tx) error {
			bucket, err := storage.bucketFor(tx, quc.Type)

			if err != nil {
				return err
			}

			if v := bucket.Get(ID); v == nil {
				return fmt.Errorf("%s is not found", quc.ID)
			}

			return bucket.Delete(ID)
		}))

	case destroy.ByEntity:
		ID, found := reflects.LookupID(quc.Entity)

		if !found {
			return iterators.Errorf("can't find ID in %s", reflects.FullyQualifiedName(quc.Entity))
		}

		return storage.Exec(destroy.ByID{Type: quc.Entity, ID: ID})

	case update.ByEntity:
		encodedID, found := reflects.LookupID(quc.Entity)

		if !found {
			return iterators.Errorf("can't find ID in %s", reflects.FullyQualifiedName(quc.Entity))
		}

		ID, err := storage.idToBytes(encodedID)

		if err != nil {
			return iterators.NewError(err)
		}

		value, err := storage.encode(quc.Entity)

		if err != nil {
			return iterators.NewError(err)
		}

		return iterators.NewError(storage.DB.Batch(func(tx *bolt.Tx) error {
			bucket, err := storage.bucketFor(tx, quc.Entity)

			if err != nil {
				return err
			}

			return bucket.Put(ID, value)
		}))

	default:
		return iterators.NewError(fmt.Errorf("%s not implemented", reflects.FullyQualifiedName(quc)))

	}
}

func (storage *Local) bucketName(e frameless.Entity) []byte {
	return []byte(reflects.FullyQualifiedName(e))
}

func (storage *Local) bucketFor(tx *bolt.Tx, e frameless.Entity) (*bolt.Bucket, error) {
	bucket := tx.Bucket(storage.bucketName(e))

	var err error

	if bucket == nil {
		err = fmt.Errorf("No entity created before with type %s", reflects.FullyQualifiedName(e))
	}

	return bucket, err
}

func (storage *Local) ensureBucketFor(tx *bolt.Tx, e frameless.Entity) (*bolt.Bucket, error) {
	return tx.CreateBucketIfNotExists(storage.bucketName(e))
}

func (storage *Local) idToBytes(ID string) ([]byte, error) {
	n, err := strconv.ParseUint(ID, 10, 64)

	if err != nil {
		return nil, fmt.Errorf("ID is not acceptable for this storage: %s", ID)
	}

	return storage.uintToBytes(n), nil
}

// uintToBytes returns an 8-byte big endian representation of v.
func (storage *Local) uintToBytes(v uint64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, v)
	return b
}

func (storage *Local) encode(e frameless.Entity) ([]byte, error) {
	buf := new(bytes.Buffer)
	enc := gob.NewEncoder(buf)
	if err := enc.Encode(e); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (storage *Local) decode(data []byte, ptr frameless.Entity) error {
	buf := bytes.NewBuffer(data)
	dec := gob.NewDecoder(buf)
	return dec.Decode(ptr)
}

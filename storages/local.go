package storages

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"fmt"
	"reflect"
	"strconv"

	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/iterators"
	"github.com/adamluzsi/frameless/queryusecases"
	"github.com/adamluzsi/frameless/reflects"
	"github.com/boltdb/bolt"
)

func NewLocal(path string) (frameless.Storage, error) {
	db, err := bolt.Open(path, 0600, nil)

	return &local{db: db}, err
}

type local struct {
	db *bolt.DB
}

// Close the local database and release the file lock
func (storage *local) Close() error {
	return storage.db.Close()
}

func (storage *local) Create(e frameless.Entity) error {
	return storage.db.Update(func(tx *bolt.Tx) error {

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

func (storage *local) Find(quc frameless.QueryUseCase) frameless.Iterator {
	switch quc := quc.(type) {
	case queryusecases.ByID:
		key, err := storage.idToBytes(quc.ID)

		if err != nil {
			return iterators.NewError(err)
		}

		entity := reflect.New(reflect.TypeOf(quc.Type)).Interface()

		err = storage.db.View(func(tx *bolt.Tx) error {
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

	case queryusecases.AllFor:
		r, w := iterators.NewPipe()

		go storage.db.View(func(tx *bolt.Tx) error {
			defer w.Close()

			bucket, err := storage.bucketFor(tx, quc.Type)
			if err != nil {
				w.Error(err)
				return err
			}

			err = bucket.ForEach(func(IDbytes, encodedEntity []byte) error {
				entity := reflect.New(reflect.TypeOf(quc.Type)).Interface()
				storage.decode(encodedEntity, entity)
				return w.Send(entity) // iterators.ErrClosed will cancel ForEach execution
			})

			if err != nil {
				w.Error(err)
				return err
			}

			return nil
		})

		return r

	default:
		return iterators.NewError(fmt.Errorf("%s not implemented", reflects.Name(quc)))

	}
}

func (storage *local) Exec(quc frameless.QueryUseCase) error {
	switch quc := quc.(type) {
	case queryusecases.DeleteByID:

		ID, err := storage.idToBytes(quc.ID)

		if err != nil {
			return err
		}

		return storage.db.Update(func(tx *bolt.Tx) error {
			bucket, err := storage.bucketFor(tx, quc.Type)

			if err != nil {
				return err
			}

			return bucket.Delete(ID)
		})

	case queryusecases.DeleteByEntity:
		ID, found := reflects.LookupID(quc.Entity)

		if !found {
			return fmt.Errorf("can't find ID in %s", reflects.Name(quc.Entity))
		}

		return storage.Exec(queryusecases.DeleteByID{Type: quc.Entity, ID: ID})

	case queryusecases.UpdateEntity:
		encodedID, found := reflects.LookupID(quc.Entity)

		if !found {
			return fmt.Errorf("can't find ID in %s", reflects.Name(quc.Entity))
		}

		ID, err := storage.idToBytes(encodedID)

		if err != nil {
			return err
		}

		value, err := storage.encode(quc.Entity)

		if err != nil {
			return err
		}

		return storage.db.Batch(func(tx *bolt.Tx) error {
			bucket, err := storage.bucketFor(tx, quc.Entity)

			if err != nil {
				return err
			}

			return bucket.Put(ID, value)
		})

	default:
		return fmt.Errorf("%s not implemented", reflects.Name(quc))

	}
}

func (storage *local) bucketName(e frameless.Entity) []byte {
	return []byte(reflects.Name(e))
}

func (storage *local) bucketFor(tx *bolt.Tx, e frameless.Entity) (*bolt.Bucket, error) {
	bucket := tx.Bucket(storage.bucketName(e))

	var err error

	if bucket == nil {
		err = fmt.Errorf("No entity created before with type %s", reflects.Name(e))
	}

	return bucket, err
}

func (storage *local) ensureBucketFor(tx *bolt.Tx, e frameless.Entity) (*bolt.Bucket, error) {
	return tx.CreateBucketIfNotExists(storage.bucketName(e))
}

func (storage *local) idToBytes(ID string) ([]byte, error) {
	n, err := strconv.ParseUint(ID, 10, 64)

	if err != nil {
		return nil, fmt.Errorf("ID is not acceptable for this storage: %s", ID)
	}

	return storage.uintToBytes(n), nil
}

// uintToBytes returns an 8-byte big endian representation of v.
func (storage *local) uintToBytes(v uint64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, v)
	return b
}

func (storage *local) encode(e frameless.Entity) ([]byte, error) {
	buf := new(bytes.Buffer)
	enc := gob.NewEncoder(buf)
	if err := enc.Encode(e); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (storage *local) decode(data []byte, ptr frameless.Entity) error {
	buf := bytes.NewBuffer(data)
	dec := gob.NewDecoder(buf)
	return dec.Decode(ptr)
}

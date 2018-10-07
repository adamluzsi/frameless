package localstorage

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"encoding/gob"
	"fmt"
	"github.com/adamluzsi/frameless/externalresources"
	"github.com/adamluzsi/frameless/queries/queryerrors"
	"github.com/adamluzsi/frameless/queries/save"
	"io/ioutil"
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

	return &Local{DB: db, CompressionLevel: gzip.DefaultCompression}, err
}

type Local struct {
	DB               *bolt.DB
	CompressionLevel int
}

// Close the Local database and release the file lock
func (storage *Local) Close() error {
	return storage.DB.Close()
}

func (storage *Local) Exec(quc frameless.Query) frameless.Iterator {
	switch quc := quc.(type) {
	case save.Entity:
		return iterators.NewError(storage.DB.Update(func(tx *bolt.Tx) error {

			if currentID, ok := externalresources.LookupID(quc.Entity); !ok || currentID != "" {
				return fmt.Errorf("entity already have an ID: %s", currentID)
			}

			bucketName := storage.BucketNameFor(quc.Entity)
			bucket, err := tx.CreateBucketIfNotExists(bucketName)

			if err != nil {
				return err
			}

			uIntID, err := bucket.NextSequence()

			if err != nil {
				return err
			}

			encodedID := strconv.FormatUint(uIntID, 10)

			if err = externalresources.SetID(quc.Entity, encodedID); err != nil {
				return err
			}

			value, err := storage.Serialize(quc.Entity)

			if err != nil {
				return err
			}

			return bucket.Put(storage.uintToBytes(uIntID), value)

		}))

	case find.ByID:
		key, err := storage.IDToBytes(quc.ID)

		if err != nil {
			return iterators.NewError(err)
		}

		entity := reflect.New(reflect.TypeOf(quc.Type)).Interface()

		err = storage.DB.View(func(tx *bolt.Tx) error {
			bucket, err := storage.BucketFor(tx, quc.Type)

			if err != nil {
				return err
			}

			encodedValue := bucket.Get(key)

			if encodedValue == nil {
				entity = nil
				return nil
			}

			return storage.Deserialize(encodedValue, entity)
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

			bucket := tx.Bucket(storage.BucketNameFor(quc.Type))

			if bucket == nil {
				return nil
			}

			if err := bucket.ForEach(func(IDbytes, encodedEntity []byte) error {
				entity := reflect.New(reflect.TypeOf(quc.Type)).Interface()
				storage.Deserialize(encodedEntity, entity)
				return w.Encode(entity) // iterators.ErrClosed will cancel ForEach execution
			}); err != nil {
				w.Error(err)
				return err
			}

			return nil
		})

		return r

	case destroy.ByID:

		ID, err := storage.IDToBytes(quc.ID)

		if err != nil {
			return iterators.NewError(err)
		}

		return iterators.NewError(storage.DB.Update(func(tx *bolt.Tx) error {
			bucket, err := storage.BucketFor(tx, quc.Type)

			if err != nil {
				return err
			}

			if v := bucket.Get(ID); v == nil {
				return fmt.Errorf("%s is not found", quc.ID)
			}

			return bucket.Delete(ID)
		}))

	case destroy.ByEntity:
		ID, found := externalresources.LookupID(quc.Entity)

		if !found || ID == "" {
			return iterators.Errorf("can't find ID in %s", reflects.FullyQualifiedName(quc.Entity))
		}

		return storage.Exec(destroy.ByID{Type: quc.Entity, ID: ID})

	case update.ByEntity:
		encodedID, found := externalresources.LookupID(quc.Entity)

		if !found || encodedID == "" {
			return iterators.Errorf("can't find ID in %s", reflects.FullyQualifiedName(quc.Entity))
		}

		ID, err := storage.IDToBytes(encodedID)

		if err != nil {
			return iterators.NewError(err)
		}

		value, err := storage.Serialize(quc.Entity)

		if err != nil {
			return iterators.NewError(err)
		}

		return iterators.NewError(storage.DB.Batch(func(tx *bolt.Tx) error {
			bucket, err := storage.BucketFor(tx, quc.Entity)

			if err != nil {
				return err
			}

			return bucket.Put(ID, value)
		}))

	default:
		return iterators.NewError(queryerrors.ErrNotImplemented)

	}
}

func (storage *Local) BucketNameFor(e frameless.Entity) []byte {
	return []byte(reflects.FullyQualifiedName(e))
}

func (storage *Local) BucketFor(tx *bolt.Tx, e frameless.Entity) (*bolt.Bucket, error) {
	bucket := tx.Bucket(storage.BucketNameFor(e))

	var err error

	if bucket == nil {
		err = fmt.Errorf("No entity created before with type %s", reflects.FullyQualifiedName(e))
	}

	return bucket, err
}

func (storage *Local) IDToBytes(ID string) ([]byte, error) {
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

func (storage *Local) Serialize(e frameless.Entity) ([]byte, error) {
	buf := new(bytes.Buffer)
	enc := gob.NewEncoder(buf)
	if err := enc.Encode(e); err != nil {
		return nil, err
	}
	return storage.compress(buf.Bytes())
}

func (storage *Local) Deserialize(CompressedAndSerialized []byte, ptr frameless.Entity) error {
	serialized, err := storage.decompress(CompressedAndSerialized)
	if err != nil {
		return err
	}
	buf := bytes.NewBuffer(serialized)
	dec := gob.NewDecoder(buf)
	return dec.Decode(ptr)
}

func (storage *Local) compress(serialized []byte) ([]byte, error) {
	buffer := bytes.NewBuffer([]byte{})
	writer, err := gzip.NewWriterLevel(buffer, storage.CompressionLevel)
	if err != nil {
		return nil, err
	}
	_, err = writer.Write(serialized)
	writer.Flush()
	writer.Close()
	return buffer.Bytes(), err
}

func (storage *Local) decompress(compressedData []byte) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(compressedData))
	if err != nil {
		fmt.Println(err)
		return nil, err
	}
	return ioutil.ReadAll(reader)
}

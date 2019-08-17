package localstorage

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/binary"
	"encoding/gob"
	"fmt"
	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/resources"
	"io/ioutil"
	"strconv"

	"github.com/adamluzsi/frameless/iterators"
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

func (storage *Local) Truncate(ctx context.Context, Type interface{}) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	return storage.DB.Update(func(tx *bolt.Tx) error {
		bucketName := storage.BucketNameFor(Type)

		if b := tx.Bucket(bucketName); b != nil {
			return tx.DeleteBucket(bucketName)
		}

		return nil
	})
}

func (storage *Local) Save(ctx context.Context, ptr interface{}) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	return storage.DB.Update(func(tx *bolt.Tx) error {

		if currentID, ok := resources.LookupID(ptr); !ok || currentID != "" {
			return fmt.Errorf("entity already have an ID: %s", currentID)
		}

		bucketName := storage.BucketNameFor(ptr)
		bucket, err := tx.CreateBucketIfNotExists(bucketName)

		if err != nil {
			return err
		}

		uIntID, err := bucket.NextSequence()

		if err != nil {
			return err
		}

		encodedID := strconv.FormatUint(uIntID, 10)

		if err = resources.SetID(ptr, encodedID); err != nil {
			return err
		}

		value, err := storage.Serialize(ptr)

		if err != nil {
			return err
		}

		return bucket.Put(storage.uintToBytes(uIntID), value)

	})
}

func (storage *Local) Update(ctx context.Context, ptr interface{}) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	encodedID, found := resources.LookupID(ptr)

	if !found || encodedID == "" {
		return fmt.Errorf("can't find ID in %s", reflects.FullyQualifiedName(ptr))
	}

	ID, err := storage.IDToBytes(encodedID)

	if err != nil {
		return err
	}

	value, err := storage.Serialize(ptr)

	if err != nil {
		return err
	}

	return storage.DB.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists(storage.BucketNameFor(ptr))

		if err != nil {
			return err
		}

		return bucket.Put(ID, value)
	})
}

func (storage *Local) Delete(ctx context.Context, Entity interface{}) error {
	ID, found := resources.LookupID(Entity)

	if !found {
		return fmt.Errorf("can't find ID in %s", reflects.FullyQualifiedName(Entity))
	}

	return storage.DeleteByID(ctx, Entity, ID)
}

func (storage *Local) FindAll(ctx context.Context, Type interface{}) frameless.Iterator {
	r, w := iterators.NewPipe()

	if err := ctx.Err(); err != nil {
		return iterators.NewError(err)
	}

	go func() {
		defer w.Close()

		err := storage.DB.View(func(tx *bolt.Tx) error {

			bucket := tx.Bucket(storage.BucketNameFor(Type))

			if bucket == nil {
				return nil
			}

			return bucket.ForEach(func(IDbytes, encodedEntity []byte) error {
				if err := ctx.Err(); err != nil {
					return err
				}

				entity := reflects.New(Type)

				if err := storage.Deserialize(encodedEntity, entity); err != nil {
					return err
				}

				return w.Encode(entity) // iterators.ErrClosed will cancel ForEach execution
			})

		})

		if err != nil {
			w.Error(err)
		}
	}()

	return r
}

func (storage *Local) FindByID(ctx context.Context, ptr interface{}, ID string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}

	var found bool

	key, err := storage.IDToBytes(ID)

	if err != nil {
		return false, nil
	}

	err = storage.DB.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(storage.BucketNameFor(ptr))

		if bucket == nil {
			found = false
			return nil
		}

		encodedValue := bucket.Get(key)
		found = encodedValue != nil

		if encodedValue == nil {
			return nil
		}

		return storage.Deserialize(encodedValue, ptr)
	})

	return found, err
}

func (storage *Local) DeleteByID(ctx context.Context, Type interface{}, ID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	ByteID, err := storage.IDToBytes(ID)

	if err != nil {
		return err
	}

	return storage.DB.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(storage.BucketNameFor(Type))

		if bucket == nil {
			return nil
		}

		if v := bucket.Get(ByteID); v == nil {
			return frameless.ErrNotFound
		}

		return bucket.Delete(ByteID)
	})

}

// Close the Local database and release the file lock
func (storage *Local) Close() error {
	return storage.DB.Close()
}

func (storage *Local) BucketNameFor(e frameless.Entity) []byte {
	return []byte(reflects.FullyQualifiedName(e))
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

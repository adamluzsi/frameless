package dataprovider

// Fetcher is an object that implements fetching logic for a given Business Entity
type Fetcher interface {
	All(slice interface{}) error
	Where(slice interface{}, query ...interface{}) error
	Find(object interface{}, ID interface{}) (isFound bool, err error)
}

type IterableFetch interface {
	IteratorForAll() Iterator
	IteratorForWhere(query ...interface{}) Iterator
}

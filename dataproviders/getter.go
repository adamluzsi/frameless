package dataproviders

// Getter interface allows to look up one specific object from a given data pile*
type Getter interface {
	// Get gets the first value associated with the given key.
	// By convention it should be a single value
	Get(key interface{}) interface{}

	// Lookup gets the first value associated with the given key.
	// If there are no values associated with the key, Get returns a second value FALSE.
	Lookup(key interface{}) (interface{}, bool)
}

type MultiGetter interface {
	// GetAll is same as Getter.Get but it should return all the values instead of the first found.
	// By convention it should return all the values or an empty list
	GetAll(key interface{}) []interface{}

	// LookupAll is the same as Getter.Lookup but return all the values associated with the given key.
	LookupAll(key interface{}) ([]interface{}, bool)
}

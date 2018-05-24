package dataproviders

// Getter interface allows to look up one specific object from a given data pile*
type Getter interface {
	// Get gets the first value associated with the given key.
	Get(key interface{}) interface{}

	// Lookup gets the first value associated with the given key.
	// If there are no values associated with the key, Get returns a second value FALSE.
	Lookup(key interface{}) (interface{}, bool)
}

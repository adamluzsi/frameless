package dataprovider

// Iterator will provide data to the user in a stream like way
//
// https://golang.org/pkg/encoding/json/#Decoder
type Iterator interface {
	// More can tell if there is still more value left or not
	More() bool
	// Decode will populate an object with values and/or return error
	Decode(interface{}) error
}

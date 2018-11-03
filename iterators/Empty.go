package iterators

// NewEmpty iterator is used to represent nil result with Null object pattern
func NewEmpty() *Empty {
	return &Empty{}
}

// Empty iterator can help achieve Null Object Pattern when no value is logically expected and iterator should be returned
type Empty struct{}

func (i *Empty) Close() error {
	return nil
}

func (i *Empty) Next() bool {
	return false
}

func (i *Empty) Err() error {
	return nil
}

func (i *Empty) Decode(interface{}) error {
	return nil
}

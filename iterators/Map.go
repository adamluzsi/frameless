package iterators

// Map allows you to do additional transformation on the values.
// This is useful in cases, where you have to alter the input value,
// or change the type all together.
// Like when you read lines from an input stream,
// and then you map the line content to a certain data structure,
// in order to not expose what steps needed in order to unserialize the input stream,
// thus protect the business rules from this information.
func Map(iter Interface, transform MapTransformFunc) *MapIter {
	return &MapIter{src: iter, transform: transform}
}

type MapTransformFunc = func(d Decoder, ptr interface{}) error

type MapIter struct {
	src       Interface
	transform MapTransformFunc
}

func (i *MapIter) Close() error {
	return i.src.Close()
}

func (i *MapIter) Next() bool {
	return i.src.Next()
}

func (i *MapIter) Err() error {
	return i.src.Err()
}

func (i *MapIter) Decode(dst interface{}) error {
	return i.transform(i.src, dst)
}

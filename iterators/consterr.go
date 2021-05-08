package iterators

type consterr string

// Error implement the error interface
func (err consterr) Error() string { return string(err) }


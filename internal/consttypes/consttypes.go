package consttypes

type String string

func (s String) String() string { return string(s) }

type Error string

// Error implement the error interface
func (err Error) Error() string { return string(err) }

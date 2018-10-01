package errtype

type Error string

func (errStr Error) Error() string { return string(errStr) }

package codec

type errcode string

func (err errcode) Error() string {
	return string(err)
}

const ErrNotSupported errcode = "ErrNotSupported"

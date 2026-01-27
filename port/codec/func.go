package codec

type MarshalerFunc func(v any) ([]byte, error)

func (fn MarshalerFunc) Marshal(v any) ([]byte, error) { return fn(v) }

type UnmarshalerFunc func(data []byte, ptr any) error

func (fn UnmarshalerFunc) Unmarshal(data []byte, ptr any) error { return fn(data, ptr) }

type EncoderFunc func(v any) error

func (fn EncoderFunc) Encode(v any) error { return fn(v) }

type DecoderFunc func(ptr any) error

func (fn DecoderFunc) Decode(ptr any) error { return fn(ptr) }

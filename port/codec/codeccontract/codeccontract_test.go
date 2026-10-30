package codeccontract_test

import (
	"encoding/json"
	"testing"

	"go.llib.dev/frameless/pkg/jsonkit"
	"go.llib.dev/frameless/port/codec"
	"go.llib.dev/frameless/port/codec/codeccontract"
)

func TestCodec_json(t *testing.T) {
	t.Run("", func(t *testing.T) {
		// encodingJSON is the reference implementation of the contract.
		var encodingJSON = struct {
			codec.MarshalerFunc
			codec.UnmarshalerFunc
		}{
			MarshalerFunc:   json.Marshal,
			UnmarshalerFunc: json.Unmarshal,
		}
		codeccontract.Codec(encodingJSON).Test(t)
	})

	t.Run("jsonkit.Codec", func(t *testing.T) {
		codeccontract.Codec(jsonkit.Codec{}, codeccontract.CodecConfig{SupportInterface: true}).Test(t)
	})
}

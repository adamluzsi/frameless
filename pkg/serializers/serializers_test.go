package serializers_test

import (
	"go.llib.dev/frameless/pkg/serializers"
	"go.llib.dev/frameless/ports/codec"
)

var _ interface {
	codec.Serializer
	// codec.ListSerializer
	// codec.ListEncoderMaker
	// codec.ListDecoderMaker
} = serializers.JSON{}

var _ serializers.Serializer = serializers.FormURLEncoder{}

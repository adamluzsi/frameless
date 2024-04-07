package serializers_test

import "go.llib.dev/frameless/pkg/serializers"

var _ interface {
	serializers.Serializer
	serializers.ListSerializer
	serializers.ListEncoderMaker
	serializers.ListDecoderMaker
} = serializers.JSON{}

var _ serializers.Serializer = serializers.FormURLEncoder{}

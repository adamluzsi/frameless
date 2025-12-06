package httpkitcodec_test

import (
	"go.llib.dev/frameless/pkg/httpkit"
	"go.llib.dev/frameless/pkg/httpkit/httpkitcodec"
)

var _ httpkit.RESTHandlerCodec[int] = httpkitcodec.JSON[int]{}
var _ httpkit.RESTClientCodec[int] = httpkitcodec.JSON[int]{}

package httpkitcodec_test

import (
	"go.llib.dev/frameless/pkg/httpkit"
	"go.llib.dev/frameless/pkg/httpkit/httpkitcodec"
)

var _ httpkit.RESTHandlerCodec[int] = httpkitcodec.JSONLines[int]{}

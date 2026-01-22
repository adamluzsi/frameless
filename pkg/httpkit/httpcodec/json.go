package httpcodec

import (
	"go.llib.dev/frameless/pkg/jsonkit"
)

type JSON[T any] jsonkit.Codec[T]

type JSONLines[T any] jsonkit.LinesCodec[T]

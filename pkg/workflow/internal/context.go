package internal

import (
	"uuid"

	"go.llib.dev/frameless/pkg/contextkit"
)

type ctxKeyProcessID struct{}

var ContextProcessID contextkit.ValueHandler[ctxKeyProcessID, uuid.UUID]

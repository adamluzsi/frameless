package internal

import (
	"go.llib.dev/frameless/pkg/contextkit"
	"go.llib.dev/frameless/pkg/uuid"
)

type ctxKeyProcessID struct{}

var ContextProcessID contextkit.ValueHandler[ctxKeyProcessID, uuid.UUID]

package serializers

import "go.llib.dev/frameless/pkg/jsonkit"

type (
	JSON       = jsonkit.Codec
	JSONStream = jsonkit.LinesSerializer
)

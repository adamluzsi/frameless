package httpkit

import (
	"mime"

	"go.llib.dev/frameless/pkg/httpkit/mediatype"
)

func lookupMediaType(mimeType string) (mediatype.MediaType, bool) {
	if mimeType == "" {
		return mimeType, false
	}
	mt, _, err := mime.ParseMediaType(mimeType)
	if err != nil || mt == "" {
		if mimeType != "" {
			return mimeType, true
		}
		return mt, false
	}
	return mt, true
}

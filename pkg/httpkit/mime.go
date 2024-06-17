package httpkit

import "mime"

func getMediaType(mimeType string) string {
	if mimeType == "" {
		return mimeType
	}
	if mt, _, err := mime.ParseMediaType(mimeType); err == nil {
		return mt
	}
	return mimeType
}

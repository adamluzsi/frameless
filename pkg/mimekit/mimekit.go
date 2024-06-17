package mimekit

import (
	"mime"
)

const (
	PlainText      = "text/plain; charset=utf-8"
	JSON           = "application/json"
	XML            = "application/xml"
	HTML           = "text/html; charset=utf-8"
	OctetStream    = "application/octet-stream"
	FormUrlencoded = "application/x-www-form-urlencoded"
)

func MediaType(mimeType string) string {
	if mimeType == "" {
		return mimeType
	}
	if mt, _, err := mime.ParseMediaType(mimeType); err == nil {
		return mt
	}
	return mimeType
}

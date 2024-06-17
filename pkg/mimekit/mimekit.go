package mimekit

import (
	"fmt"
	"mime"
	"strings"
)

// MIMEType or Multipurpose Internet Mail Extensions is an internet standard
// that extends the original email protocol to support non-textual content,
// such as images, audio files, and binary data.
//
// It was first defined in RFC 1341 and later updated in RFC 2045.
// MIMEType allows for the encoding different types of data using a standardised format
// that can be transmitted over email or other internet protocols.
// This makes it possible to send and receive messages with a variety of content,
// such as text, images, audio, and video, in a consistent way across different mail clients and servers.
//
// The MIMEType type is an essential component of this system, as it specifies the format of the data being transmitted.
// A MIMEType type consists of two parts: the type and the subtype, separated by a forward slash (`/`).
// The type indicates the general category of the data, such as `text`, `image`, or `audio`.
// The subtype provides more information about the specific format of the data,
// such as `plain` for plain text or `jpeg` for JPEG images.
// Today MIMEType is not only used for email but also for other internet protocols, such as HTTP,
// where it is used to specify the format of data in web requests and responses.
//
// MIMEType type is commonly used in RESTful APIs as well.
// In an HTTP request or response header, the Content-Type field specifies the MIMEType type of the entity body.
type MIMEType string

var _ mime.WordDecoder

const (
	PlainText      MIMEType = "text/plain"
	JSON           MIMEType = "application/json"
	XML            MIMEType = "application/xml"
	HTML           MIMEType = "text/html; charset=UTF-8"
	OctetStream    MIMEType = "application/octet-stream"
	FormUrlencoded MIMEType = "application/x-www-form-urlencoded"
)

func (ct MIMEType) WithCharset(charset string) MIMEType {
	const attrKey = "charset"
	if strings.Contains(string(ct), attrKey) {
		var parts []string
		for _, pt := range strings.Split(string(ct), ";") {
			if !strings.Contains(pt, attrKey) {
				parts = append(parts, pt)
			}
		}
		ct = MIMEType(strings.Join(parts, ";"))
	}
	return MIMEType(fmt.Sprintf("%s; %s=%s", ct, attrKey, charset))
}

func (ct MIMEType) Base() MIMEType {
	for _, pt := range strings.Split(string(ct), ";") {
		return MIMEType(pt)
	}
	return ct
}

func (ct MIMEType) String() string { return string(ct) }

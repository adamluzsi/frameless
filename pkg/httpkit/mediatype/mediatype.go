package mediatype

const (
	PlainText      MediaType = "text/plain; charset=utf-8"
	JSON           MediaType = "application/json"
	XML            MediaType = "application/xml"
	HTML           MediaType = "text/html; charset=utf-8"
	OctetStream    MediaType = "application/octet-stream"
	FormUrlencoded MediaType = "application/x-www-form-urlencoded"
)

type MediaType = string

package logdto

type Error struct {
	Message string `json:"message"`
	Code    string `json:"code,omitempty"   xml:"code,omitempty"`
	Detail  string `json:"detail,omitempty" xml:"detail,omitempty"`
}

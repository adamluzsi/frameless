package openapiv3

import (
	"net/http"
	"time"
)

type Spec struct {
	Openapi string    `json:"openapi" yaml:"openapi"`
	Info    SpecInfo  `json:"info" yaml:"info"`
	Paths   SpecPaths `json:"paths" yaml:"paths"`
}

// httpMethod
type httpMethod string

const (
	MethodGet     httpMethod = http.MethodGet
	MethodHead    httpMethod = http.MethodHead
	MethodPost    httpMethod = http.MethodPost
	MethodPut     httpMethod = http.MethodPut
	MethodPatch   httpMethod = http.MethodPatch
	MethodDelete  httpMethod = http.MethodDelete
	MethodConnect httpMethod = http.MethodConnect
	MethodOptions httpMethod = http.MethodOptions
	MethodTrace   httpMethod = http.MethodTrace
)

type SpecPaths map[string]SpecMethods

type SpecMethods map[httpMethod]any

type SpecOperation struct {
	OperationId string `json:"operationId" yaml:"operationId"`
	Summary     string `json:"summary" yaml:"summary"`
	Responses   struct {
		Field1 struct {
			Description string `json:"description" yaml:"description"`
			Content     struct {
				ApplicationJson struct {
					Examples struct {
						Foo struct {
							Value struct {
								Versions []struct {
									Status  string    `json:"status" yaml:"status"`
									Updated time.Time `json:"updated" yaml:"updated"`
									Id      string    `json:"id" yaml:"id"`
									Links   []struct {
										Href string `json:"href" yaml:"href"`
										Rel  string `json:"rel" yaml:"rel"`
									} `json:"links" yaml:"links"`
								} `json:"versions" yaml:"versions"`
							} `json:"value" yaml:"value"`
						} `json:"foo" yaml:"foo"`
					} `json:"examples" yaml:"examples"`
				} `json:"application/json" yaml:"application/json"`
			} `json:"content" yaml:"content"`
		} `json:"200" yaml:"200"`
		Field2 struct {
			Description string `json:"description" yaml:"description"`
			Content     struct {
				ApplicationJson struct {
					Examples struct {
						Foo struct {
							Value string `json:"value" yaml:"value"`
						} `json:"foo" yaml:"foo"`
					} `json:"examples" yaml:"examples"`
				} `json:"application/json" yaml:"application/json"`
			} `json:"content" yaml:"content"`
		} `json:"300" yaml:"300"`
	} `json:"responses,omitempty" yaml:"responses"`

	OperationId string `json:"operationId" yaml:"operationId"`
	Summary     string `json:"summary" yaml:"summary"`
	Responses   struct {
		Field1 struct {
			Description string `json:"description" yaml:"description"`
			Content     struct {
				ApplicationJson struct {
					Examples struct {
						Foo struct {
							Value struct {
								Version struct {
									Status     string    `json:"status" yaml:"status"`
									Updated    time.Time `json:"updated" yaml:"updated"`
									MediaTypes []struct {
										Base string `json:"base" yaml:"base"`
										Type string `json:"type" yaml:"type"`
									} `json:"media-types" yaml:"media-types"`
									Id    string `json:"id" yaml:"id"`
									Links []struct {
										Href string `json:"href" yaml:"href"`
										Rel  string `json:"rel" yaml:"rel"`
										Type string `json:"type,omitempty" yaml:"type,omitempty"`
									} `json:"links" yaml:"links"`
								} `json:"version" yaml:"version"`
							} `json:"value" yaml:"value"`
						} `json:"foo" yaml:"foo"`
					} `json:"examples" yaml:"examples"`
				} `json:"application/json" yaml:"application/json"`
			} `json:"content" yaml:"content"`
		} `json:"200" yaml:"200"`
		Field2 struct {
			Description string `json:"description" yaml:"description"`
			Content     struct {
				ApplicationJson struct {
					Examples struct {
						Foo struct {
							Value struct {
								Version struct {
									Status     string    `json:"status" yaml:"status"`
									Updated    time.Time `json:"updated" yaml:"updated"`
									MediaTypes []struct {
										Base string `json:"base" yaml:"base"`
										Type string `json:"type" yaml:"type"`
									} `json:"media-types" yaml:"media-types"`
									Id    string `json:"id" yaml:"id"`
									Links []struct {
										Href string `json:"href" yaml:"href"`
										Rel  string `json:"rel" yaml:"rel"`
										Type string `json:"type,omitempty" yaml:"type,omitempty"`
									} `json:"links" yaml:"links"`
								} `json:"version" yaml:"version"`
							} `json:"value" yaml:"value"`
						} `json:"foo" yaml:"foo"`
					} `json:"examples" yaml:"examples"`
				} `json:"application/json" yaml:"application/json"`
			} `json:"content" yaml:"content"`
		} `json:"203" yaml:"203"`
	} `json:"responses" yaml:"responses"`
}

type SpecInfo struct {
	Title   string `json:"title" yaml:"title"`
	Version string `json:"version" yaml:"version"`
}

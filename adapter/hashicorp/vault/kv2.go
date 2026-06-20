package vault

import "encoding/json"

// KVResponseV2 represents the response from a  KV v2 secrets engine read operation.
type KVResponseV2 struct {
	RequestID     string    `json:"request_id"`
	LeaseID       string    `json:"lease_id"`
	Renewable     bool      `json:"renewable"`
	LeaseDuration int       `json:"lease_duration"`
	Data          KV2Data   `json:"data"`
	WrapInfo      *WrapInfo `json:"wrap_info"`
	Warnings      []string  `json:"warnings"`
	Auth          *AuthInfo `json:"auth"`
}

// KV2Data represents the nested data structure in KV v2 responses.
type KV2Data struct {
	Data     json.RawMessage `json:"data"`
	Metadata Metadata        `json:"metadata"`
}

// Metadata represents the metadata associated with a secret version.
type Metadata struct {
	CreatedTime    string            `json:"created_time"`
	CustomMetadata map[string]string `json:"custom_metadata"`
	DeletionTime   string            `json:"deletion_time"`
	Destroyed      bool              `json:"destroyed"`
	Version        int               `json:"version"`
}

// WrapInfo represents response wrapping information.
type WrapInfo struct {
	Token           string `json:"token"`
	Accessor        string `json:"accessor"`
	TTL             int    `json:"ttl"`
	Algorithm       string `json:"algorithm"`
	WrappedCACert   string `json:"wrapped_ca_cert"`
	Recipient       string `json:"recipient"`
	SecondAlgorithm string `json:"second_algorithm"`
}

// AuthInfo represents authentication information (if present in the response).
type AuthInfo struct {
	ClientToken   string            `json:"client_token"`
	Accessor      string            `json:"accessor"`
	Policies      []string          `json:"policies"`
	Metadata      map[string]string `json:"metadata"`
	LeaseDuration int               `json:"lease_duration"`
	Renewable     bool              `json:"renewable"`
	EntityID      string            `json:"entity_id"`
	TokenType     string            `json:"token_type"`
	Orphan        bool              `json:"orphan"`
	MfaRequire    bool              `json:"mfa_require"`
	WrapInfo      *WrapInfo         `json:"wrap_info"`
}

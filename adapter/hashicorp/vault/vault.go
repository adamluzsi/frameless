package vault

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"iter"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/vault-client-go"
	"github.com/hashicorp/vault-client-go/schema"

	"go.llib.dev/frameless/pkg/cache"
	"go.llib.dev/frameless/pkg/devops/health"
	"go.llib.dev/frameless/pkg/dtokit"
	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/logging"
	"go.llib.dev/frameless/pkg/pathkit"
	"go.llib.dev/frameless/pkg/zerokit"
	"go.llib.dev/frameless/port/crud"
	"go.llib.dev/frameless/port/crud/extid"
	"go.llib.dev/testcase/pp"
)

type Client struct {
	BaseURL string `env:"VAULT_URL" required:"true"`
	// Mount here represents the used vault mount/secret-engine for storing secrets,
	// but it is not part of the Client itself, but used as a common config!
	Mount string `env:"VAULT_MOUNT" required:"false"`

	Client     *vault.Client
	HTTPClient *http.Client

	GetToken func(ctx context.Context) (string, error)

	rwm sync.RWMutex
}

func (c *Client) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	const defaultTimeout = 30 * time.Second
	return &http.Client{Timeout: defaultTimeout}
}

func (c *Client) vaultClient() (*vault.Client, error) {
	return vault.New(
		vault.WithAddress(c.BaseURL),
		vault.WithRequestTimeout(requestTimeout),
		func(cc *vault.ClientConfiguration) error {
			if c.HTTPClient != nil {
				cc.HTTPClient = c.HTTPClient
			}
			cc.HTTPClient

			if c.HTTPRoundTripperFactory == nil {
				return nil
			}
			transport := cc.HTTPClient.Transport
			if transport == nil {
				return fmt.Errorf("*vault.ClientConfiguration.HTTPClient.Transport was unexpectedly nil")
			}
			cc.HTTPClient.Transport = c.HTTPRoundTripperFactory(transport)
			return nil
		},
	)
}

func (c *Client) HealthCheck(ctx context.Context) health.Report {
	ctx = withLoggingFields(ctx)

	var report health.Report
	report.Name = "vault"

	var opts []vault.RequestOption
	if err := addToken(ctx, c, &opts); err != nil {
		return report.WithIssue(ctx, health.Issue{
			Code:    "invalid-vault-token-configuration",
			Message: err.Error(),
			Causes:  health.Unknown,
		})
	}

	resp, err := c.Client.System.ReadHealthStatus(ctx, opts...)

	if err != nil {
		return report.WithIssue(ctx, health.Issue{
			Code:    "health-status-not-reachable",
			Message: err.Error(),
			Causes:  health.Degraded,
		})
	}

	if resp.Data != nil {
		report.Details = resp.Data
	}

	if len(resp.Warnings) == 0 {
		report.Status = health.Up
	} else {
		report.Status = health.Degraded
	}

	return report
}

// Repository
//
// Known issues: due to how the HashiCorp Vault client works, int64 values are not encodable.
type Repository[ENT any, ID ~string] struct {
	Client *Client
	// Mapper is the mapping logic about how to map back and forth
	// a domain entity and a vault JSON DTO.
	Mapper dtokit.Mapper[ENT]
	// MountPoint is the path that leads to a key-value/tree store on the Vault side.
	//
	// In local testing, the default usually is "secret".
	// It differs on different setups, but the behaviour of the store itself will be the same.
	MountPoint string
	// BasePath is the predeceasing path of a given entity stored in the Vault KeyValue store.
	//
	// BasePath enables mimicking separate DB tables for different entity records.
	// For example, for a specific entity type, it could be "entities".
	BasePath string
	// DeletePermanently tells the Vault Repository to not just delete the current record, but delete it permanently.
	// Warning, using this flag could lead to loss of data, if deletion is intended to be used as a form of archiving.
	DeletePermanently bool
	// IDA is an ID accessor for ENT, that can retrieve or set a given ID value on a given ENT value.
	// This is an optional field, and by default it will check the ext tag on a struct fields.
	IDA extid.Accessor[ENT, ID]

	MakeID func(ctx context.Context) (ID, error)
}

func (r Repository[ENT, ID]) Create(ctx context.Context, ptr *ENT) error {
	ctx = withLoggingFields(ctx)

	dto, err := r.Mapper.MapToIDTO(ctx, *ptr)
	if err != nil {
		return errorkit.F("failed to map %T to its dto format: %w", ptr, err)
	}

	data, err := toVaultDataDTO(dto)
	if err != nil {
		return err
	}

	id, _ := r.IDA.Lookup(*ptr)

	if zerokit.IsZero(id) {
		if r.MakeID == nil {
			return fmt.Errorf("missing hashicorpvault.Repository#MakeID")
		}

		var err error
		id, err = r.MakeID(ctx)
		if err != nil {
			return fmt.Errorf("error from hashicorpvault.Repository#MakeID: %w", err)
		}

		if err := r.IDA.Set(ptr, id); err != nil {
			return err
		}
	}

	requestOptions, err := r.requestOptions(ctx)
	if err != nil {
		return err
	}

	pp.PP(data)

	_, err = r.Client.Secrets.KvV2Write(ctx, r.getVaultPath(id),
		schema.KvV2WriteRequest{
			Data: data,
		}, requestOptions...)

	if err != nil {
		return err
	}

	return nil
}

func (r Repository[ENT, ID]) requestOptions(ctx context.Context) ([]vault.RequestOption, error) {
	var opts = []vault.RequestOption{}

	if len(r.MountPoint) != 0 {
		opts = append(opts, vault.WithMountPath(trimSlash(r.MountPoint)))
	}

	if err := addToken(ctx, r.Client, &opts); err != nil {
		return nil, err
	}

	return opts, nil
}

func addToken(ctx context.Context, c *Client, opts *[]vault.RequestOption) error {
	if c.GetToken == nil {
		return nil
	}
	token, err := c.GetToken(ctx)
	if err != nil {
		return err
	}
	*opts = append(*opts, vault.WithToken(token))
	return nil
}

func (r Repository[ENT, ID]) FindAll(ctx context.Context) iter.Seq2[ENT, error] {
	ctx = withLoggingFields(ctx)
	return iterkit.From(func(yield func(ENT) bool) error {
		requestOptions, err := r.requestOptions(ctx)
		if err != nil {
			return err
		}
		response, err := r.Client.Secrets.KvV2List(ctx, r.basePath(), requestOptions...)
		if err != nil {
			var vaultResponseError *vault.ResponseError
			if errors.As(err, &vaultResponseError) {
				if vaultResponseError.Errors == nil && vaultResponseError.StatusCode == 404 {
					// No entities found at this path, return empty iterator
					return nil
				}
			}
			return err
		}
		for _, id := range response.Data.Keys {
			if id == "metadata" { // TODO: FIXME: test me please
				continue
			}
			// URL-decode the ID since it was encoded when stored
			decodedID, err := url.PathUnescape(id)
			if err != nil {
				// If decoding fails, use the original ID (shouldn't happen in normal cases)
				decodedID = id
			}
			ent, found, err := r.FindByID(ctx, ID(decodedID))
			if err != nil {
				return err
			}
			if !found {
				// race condition, we knew in the past that there is a entity with this ID,
				// but it was deleted right after the received back the KvV2List result.
				continue
			}
			if !yield(ent) {
				// !yield is used because yield reports back if iteration is expected to continue,
				// if not, then yield will be false, meaning !false -> true
				// so we stop the iteration by returning early.
				//
				// If we would call yield one more time after it returned with false already,
				// we would get a runtime error.
				return nil
			}
		}

		return nil
	})
}

// FindByID attempts to find an entity using its ID.
// It will inform you if it successfully located the entity or if there was an unexpected issue during the process.
// Instead of using an error to represent a "not found" situation,
// a return boolean value is used to provide this information explicitly.
//
// Why the return signature includes a found bool value?
//
// This approach serves two key purposes.
// First, it ensures that the go-vet tool checks if the 'found' boolean variable is reviewed before using the entity.
// Second, it enhances readability and demonstrates the function's cyclomatic complexity.
//
//	total: 2^(n+1+1)
//	  -> found/bool 2^(n+1)  | An entity might be found or not.
//	  -> error 2^(n+1)       | An error might occur or not.
//
// Additionally, this method prevents returning an initialized pointer type with no value,
// which could lead to a runtime error if a valid but nil pointer is given to an interface variable type.
//
//	(MyInterface)((*T)(nil)) != nil
//
// Similar approaches can be found in the standard library,
// such as SQL null value types and environment lookup in the os package.
func (r Repository[ENT, ID]) FindByID(ctx context.Context, id ID) (ENT, bool, error) {
	ctx = withLoggingFields(ctx)
	ctx = r.withID(ctx, id)

	// Build the Vault KV v2 read endpoint URL
	// Vault KV v2 read endpoint: /v1/{mount}/data/{path}
	mountPath := trimSlash(r.MountPoint)
	if mountPath == "" {
		mountPath = "kv-v2"
	}
	vaultPath := r.getVaultPath(id)
	endpointURL := fmt.Sprintf("%s/v1/%s/data/%s", strings.TrimRight(r.Client.Config.BaseURL, "/"), mountPath, vaultPath)

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpointURL, nil)
	if err != nil {
		var zero ENT
		return zero, false, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Add authentication token if available
	if r.Client.GetToken != nil {
		token, err := r.Client.GetToken(ctx)
		if err != nil {
			var zero ENT
			return zero, false, fmt.Errorf("failed to get vault token: %w", err)
		}
		req.Header.Set("X-Vault-Token", token)
	}

	// Execute HTTP request
	resp, err := r.Client.HTTPClient.Do(req)
	if err != nil {
		var zero ENT
		return zero, false, fmt.Errorf("failed to execute HTTP request: %w", err)
	}
	defer resp.Body.Close()

	// Handle HTTP status codes
	if resp.StatusCode == http.StatusNotFound {
		var zero ENT
		return zero, false, nil
	}
	if resp.StatusCode != http.StatusOK {
		var zero ENT
		body, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return zero, false, fmt.Errorf("vault request failed with status %d (error reading response body: %v)", resp.StatusCode, readErr)
		}
		return zero, false, fmt.Errorf("vault request failed with status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body) // TODO: iokit.ReadAllWithLimit
	if err != nil {
		var zero ENT
		return zero, false, fmt.Errorf("failed to read vault response: %w", err)
	}
	_ = resp.Body.Close()

	var kvResp KVResponseV2

	if err := json.Unmarshal(body, &kvResp); err != nil {
		var zero ENT
		return zero, false, fmt.Errorf("failed to decode vault response: %w", err)
	}

	var dtoPtr = r.Mapper.NewDTO()

	if err := json.Unmarshal(kvResp.Data.Data, dtoPtr); err != nil {
		var zero ENT
		return zero, false, fmt.Errorf("error unmarshaling kv v2 data.data into %T: %w", dtoPtr, err)
	}

	ent, err := r.Mapper.MapFromDTO(ctx, dtoPtr)
	if err != nil {
		var zero ENT
		return zero, false, fmt.Errorf("error unmarshaling kv v2 data.data into %T: %w", dtoPtr, err)
	}

	if err := r.IDA.Set(&ent, id); err != nil {
		var zero ENT
		return zero, false, fmt.Errorf("error unmarshaling kv v2 data.data into %T: %w", dtoPtr, err)
	}

	return ent, true, nil

	// // Extract the nested data field
	// dataFieldData, ok := rawResponse["data"]
	// if !ok {
	// 	var zero ENT
	// 	return zero, false, fmt.Errorf("vault response missing 'data' field")
	// }

	// var dataField map[string]json.RawMessage
	// if err := json.Unmarshal(dataFieldData, &dataField); err != nil {
	// 	var zero ENT
	// 	return zero, false, fmt.Errorf("vault response 'data' field is not a map")
	// }

	// // Extract the actual secret data from the nested 'data' field
	// secretDataRaw, ok := dataField["data"]
	// if !ok {
	// 	var zero ENT
	// 	return zero, false, fmt.Errorf("vault response 'data.data' field is missing")
	// }

	// // Map the DTO to the entity type
	// dtoPtr := r.Mapper.NewDTO()
	// if err := json.Unmarshal(secretDataRaw, &dtoPtr); err != nil {
	// 	var zero ENT
	// 	return zero, false, fmt.Errorf("failed to unmarshal 'data.data' field's content: %w", err)
	// }

	// ent, err := r.Mapper.MapFromDTO(ctx, dtoPtr)
	// if err != nil {
	// 	var zero ENT
	// 	return zero, false, err
	// }

	// if err := r.IDA.Set(&ent, id); err != nil {
	// 	var zero ENT
	// 	return zero, false, err
	// }

	// return ent, true, nil
}

func (r Repository[ENT, ID]) DeleteByID(ctx context.Context, id ID) error {
	ctx = withLoggingFields(ctx)
	if r.DeletePermanently {
		return r.PermanentDeleteByID(ctx, id)
	}

	_, found, err := r.FindByID(ctx, id)
	if err != nil {
		return errorkit.F("failed to look up resource by %v id in vault: %w", id, err)
	}
	if !found {
		return crud.ErrNotFound.F("id=%v", id)
	}
	requestOptions, err := r.requestOptions(ctx)
	if err != nil {
		return err
	}
	vaultRecordPath := r.getVaultPath(id)
	_, err = r.Client.Secrets.KvV2Delete(ctx, vaultRecordPath, requestOptions...)
	return err
}

func (r Repository[ENT, ID]) PermanentDeleteByID(ctx context.Context, id ID) error {
	ctx = withLoggingFields(ctx)
	requestOptions, err := r.requestOptions(ctx)
	if err != nil {
		return err
	}
	vaultRecordPath := r.getVaultPath(id)

	if _, err := r.Client.Secrets.KvV2ReadMetadata(ctx, vaultRecordPath, requestOptions...); err != nil {
		var vaultResponseError *vault.ResponseError
		if errors.As(err, &vaultResponseError) {
			if vaultResponseError.StatusCode == http.StatusNotFound {
				return crud.ErrNotFound
			}
		}
		return err
	}

	_, err = r.Client.Secrets.KvV2DeleteMetadataAndAllVersions(ctx, vaultRecordPath, requestOptions...)
	return err
}

// Save implements crud.Saver interface.
// It creates the entity if it doesn't exist, or updates it if it does.
func (r Repository[ENT, ID]) Save(ctx context.Context, ptr *ENT) error {
	ctx = withLoggingFields(ctx)

	id, found := r.IDA.Lookup(*ptr)
	if !found || zerokit.IsZero(id) {
		// Entity has no valid ID, treat as create
		return r.Create(ctx, ptr)
	}

	// Check if entity exists
	_, exists, err := r.FindByID(ctx, id)
	if err != nil {
		return err
	}

	if exists {
		// Entity exists, update it
		return r.Update(ctx, ptr)
	}

	// Entity doesn't exist, create it
	return r.Create(ctx, ptr)
}

// Update updates an existing entity. Returns an error if the entity doesn't exist.
func (r Repository[ENT, ID]) Update(ctx context.Context, ptr *ENT) error {
	ctx = withLoggingFields(ctx)

	id, found := r.IDA.Lookup(*ptr)
	if !found || zerokit.IsZero(id) {
		return crud.ErrNotFound.F("entity has no valid ID")
	}

	// Verify entity exists before updating
	_, exists, err := r.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if !exists {
		return crud.ErrNotFound.F("entity with id=%v not found", id)
	}

	// Perform the update by creating (which overwrites in Vault KV v2)
	return r.Create(ctx, ptr)
}

// DeleteAll implements crud.AllDeleter interface.
// It deletes all entities in the repository.
func (r Repository[ENT, ID]) DeleteAll(ctx context.Context) error {
	ctx = withLoggingFields(ctx)

	// Collect all IDs first to avoid iteration issues
	var ids []ID
	for ent, err := range r.FindAll(ctx) {
		if err != nil {
			return err
		}

		id, ok := r.IDA.Lookup(ent)
		if !ok {
			continue
		}
		ids = append(ids, id)
	}

	// Delete each entity
	for _, id := range ids {
		if err := r.DeleteByID(ctx, id); err != nil {
			return err
		}
	}

	return nil
}

type ctxKeyIDKey[KeyID any] struct{}

func (r Repository[ENT, ID]) withID(ctx context.Context, id ID) context.Context {
	return context.WithValue(ctx, ctxKeyIDKey[ID]{}, id)
}

type vaultDTOFormat = map[string]any

func toVaultDataDTO(key any) (vaultDTOFormat, error) {
	data, err := json.Marshal(key)
	if err != nil {
		return nil, err
	}
	var writeDTO map[string]json.RawMessage
	if err := json.Unmarshal(data, &writeDTO); err != nil {
		return nil, err
	}

	var out = make(vaultDTOFormat, len(writeDTO))
	for k, v := range writeDTO {
		out[k] = v
	}

	pp.PP(key, out)
	return out, err
}

func dtoToKey(dto vaultDTOFormat, pointerToKeyDTO any) error {
	data, err := json.Marshal(dto)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, pointerToKeyDTO)
}

func (r Repository[ENT, ID]) basePath() string {
	return trimSlash(r.BasePath)
}

func (r Repository[ENT, ID]) getVaultPath(id ID) string {
	// URL-encode the ID to handle special characters like '/' that could break the path
	encodedID := url.PathEscape(string(id))
	return trimSlash(pathkit.Join(r.BasePath, encodedID))
}

func trimSlash(s string) string {
	const slash = "/"
	for strings.HasPrefix(s, slash) {
		s = strings.TrimPrefix(s, slash)
	}
	for strings.HasSuffix(s, slash) {
		s = strings.TrimSuffix(s, slash)
	}
	return s
}

type KubernetesIntegration struct {
	Client                *Client
	GetKubernetesJWTToken func(ctx context.Context) (string, error)

	ServiceAccountName string `env:"VAULT_KUBERNETES_JWT_INTEGRATION_SERVICE_ACCOUNT_NAME" required:"true"`
	MountPoint         string `env:"VAULT_KUBERNETES_JWT_INTEGRATION_MOUNT" required:"true"`
	RoleName           string `env:"VAULT_KUBERNETES_JWT_INTEGRATION_ROLE" required:"true"`
}

type JWTLoginData struct {
	ClientToken   string
	CreatedAt     time.Time
	LeaseDuration time.Duration
}

func (ki KubernetesIntegration) CreateToken(ctx context.Context) (*JWTLoginData, error) {
	ctx = withLoggingFields(ctx)
	createdAt := time.Now()

	k8sJWTToken, err := ki.GetKubernetesJWTToken(ctx)
	if err != nil {
		return nil, err
	}

	req := schema.JwtLoginRequest{
		Jwt:  k8sJWTToken,
		Role: ki.RoleName,
	}
	resp, err := ki.Client.Auth.JwtLogin(ctx, req, vault.WithMountPath(ki.MountPoint))
	if err != nil {
		return nil, errorkit.WithTrace(err)
	}
	var warning string
	if 0 < len(warning) {
		warning = "\n" + strings.Join(resp.Warnings, "\n")
	}

	if resp.Auth == nil {
		return nil, errorkit.F("missing auth data in vault JWT Login response %s", warning)
	}

	clientToken := resp.Auth.ClientToken

	if len(clientToken) == 0 {
		return nil, errorkit.F("Vault JWT Login replied with empty client token %s", warning)
	}

	var duration = time.Duration(resp.Auth.LeaseDuration) * time.Second

	return &JWTLoginData{
		ClientToken:   clientToken,
		CreatedAt:     createdAt,
		LeaseDuration: duration,
	}, nil
}

type KubernetesVaultTokenCache struct {
	KubernetesIntegration KubernetesIntegration

	c cache.RefreshCache[*JWTLoginData]
	i sync.Once
}

func (c *KubernetesVaultTokenCache) init() {
	c.i.Do(func() {
		c.c.Refresh = func(ctx context.Context) (*JWTLoginData, error) {

			return c.KubernetesIntegration.CreateToken(ctx)
		}
		c.c.IsExpired = func(ctx context.Context, v *JWTLoginData) (bool, error) {
			now := time.Now()
			//  ---|--------|---------|----->
			//  created -> now -> deadline
			deadline := v.CreatedAt.Add(v.LeaseDuration)
			// remove 5 sec buffer time to avoid accepting tokens
			// which are about to expire in the next 5 seconds
			deadline = deadline.Add(time.Second * 5 * -1)
			// expired == now is after deadline
			return now.After(deadline), nil
		}
	})
}

func (c *KubernetesVaultTokenCache) GetToken(ctx context.Context) (string, error) {
	ctx = withLoggingFields(ctx)
	c.init()
	jwtLogin, err := c.c.Load(ctx)
	if err != nil {
		return "", err
	}
	return jwtLogin.ClientToken, nil
}

func withLoggingFields(ctx context.Context) context.Context {
	const (
		pkg  = "hashicorpvault"
		name = "HashiCorp Vault"
	)
	return logging.ContextWith(ctx, logging.Fields{
		"adapter-package": pkg,
		"adapter-name":    name,
	})
}

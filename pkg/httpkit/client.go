package httpkit

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"go.llib.dev/frameless/pkg/pathkit"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/pkg/retry"
	"go.llib.dev/frameless/pkg/serializers"
	"go.llib.dev/frameless/pkg/zerokit"
	"go.llib.dev/frameless/ports/crud"
	"go.llib.dev/frameless/ports/crud/extid"
	"go.llib.dev/frameless/ports/iterators"
)

type RestClient[Entity, ID any] struct {
	BaseURL     string
	HTTPClient  *http.Client
	MIMEType    string
	Mapping     Mapping[Entity]
	Serializer  RestClientSerializer
	IDConverter idConverter[ID]
	LookupID    crud.LookupIDFunc[Entity, ID]
}

type RestClientSerializer interface {
	serializers.Serializer
	serializers.ListDecoderMaker
}

func (r RestClient[Entity, ID]) Create(ctx context.Context, ptr *Entity) error {
	if ptr == nil {
		return fmt.Errorf("nil pointer (%s) received",
			reflectkit.TypeOf[Entity]().String())
	}

	baseURL, err := r.getBaseURL(ctx)
	if err != nil {
		return err
	}

	mimeType := r.getMIMEType()
	ser := r.getSerializer(mimeType)
	mapping := r.getMapping()

	dto, err := mapping.toDTO(ctx, *ptr)
	if err != nil {
		return err
	}

	data, err := ser.Marshal(dto)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, pathkit.Join(baseURL, "/"), bytes.NewReader(data))
	if err != nil {
		return err
	}

	req.Header.Set(headerKeyContentType, mimeType)
	req.Header.Set(headerKeyAccept, mimeType)

	resp, err := r.httpClient().Do(req)
	if err != nil {
		return err
	}

	responseBody, err := bodyReadAll(resp.Body, DefaultBodyReadLimit)
	if err != nil {
		return err
	}

	if !statusOK(resp) {
		switch resp.StatusCode {
		case http.StatusConflict:
			return crud.ErrAlreadyExists
		default:
			return makeClientErrUnexpectedResponse(req, resp, responseBody)
		}
	}

	dtoPtr := mapping.newDTO()
	if err := ser.Unmarshal(responseBody, dtoPtr); err != nil {
		return err
	}

	got, err := mapping.toEnt(ctx, dtoPtr)
	if err != nil {
		return err
	}

	*ptr = got
	return nil
}

func (r RestClient[Entity, ID]) FindAll(ctx context.Context) iterators.Iterator[Entity] {
	baseURL, err := r.getBaseURL(ctx)
	if err != nil {
		return iterators.Error[Entity](err)
	}

	//mapping := r.getMapping()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pathkit.Join(baseURL, "/"), nil)
	if err != nil {
		return iterators.Error[Entity](err)
	}

	req.Header.Set(headerKeyContentType, r.getMIMEType())
	req.Header.Set(headerKeyAccept, r.getMIMEType())

	resp, err := r.httpClient().Do(req)
	if err != nil {
		return iterators.Error[Entity](err)
	}

	mapping := r.getMapping()

	mimeType, ser, ok := r.contentTypeBasedSerializer(resp)
	if !ok {
		return iterators.Error[Entity](fmt.Errorf("no serializer configured for response content type: %s", mimeType))
	}

	dm, ok := ser.(serializers.ListDecoderMaker)
	if !ok {
		return iterators.Error[Entity](fmt.Errorf("no serializer found for the received mime type"))
	}

	dec := dm.MakeListDecoder(resp.Body)

	return iterators.Func[Entity](func() (v Entity, ok bool, err error) {
		if !dec.Next() {
			return v, false, dec.Err()
		}

		ptr := mapping.newDTO()
		if err := dec.Decode(ptr); err != nil {
			return v, false, err
		}

		ent, err := mapping.toEnt(ctx, ptr)
		if err != nil {
			return v, false, err
		}

		return ent, true, nil
	}, iterators.OnClose(dec.Close))
}

func (r RestClient[Entity, ID]) contentTypeBasedSerializer(resp *http.Response) (string, Serializer, bool) {
	mt := string(resp.Header.Get("Content-Type"))
	ser, ok := r.lookupSerializer(mt)
	if !ok && r.Serializer != nil {
		ser, ok = r.Serializer, true
	}
	return mt, ser, ok
}

func (r RestClient[Entity, ID]) getResponseMimeType(resp *http.Response) string {
	ct := resp.Header.Get(headerKeyContentType)
	if ct != "" {
		return getMediaType(ct)
	}
	return r.MIMEType
}

func (r RestClient[Entity, ID]) FindByID(ctx context.Context, id ID) (ent Entity, found bool, err error) {
	mapping := r.getMapping()

	pathParamID, err := r.getIDConverter().FormatID(id)
	if err != nil {
		return ent, false, err
	}

	baseURL, err := r.getBaseURL(ctx)
	if err != nil {
		return ent, false, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pathkit.Join(baseURL, pathParamID), nil)
	if err != nil {
		return ent, false, err
	}

	req.Header.Set(headerKeyContentType, r.getMIMEType())
	req.Header.Set(headerKeyAccept, r.getMIMEType())

	resp, err := r.httpClient().Do(req)
	if err != nil {
		return ent, false, err
	}

	responseBody, err := bodyReadAll(resp.Body, DefaultBodyReadLimit)
	if err != nil {
		return ent, false, err
	}

	if resp.StatusCode == http.StatusNotFound {
		return ent, false, nil
	}

	if !statusOK(resp) {
		return ent, false, makeClientErrUnexpectedResponse(req, resp, responseBody)
	}

	dtoPtr := mapping.newDTO()
	mimeType, ser, ok := r.contentTypeBasedSerializer(resp)
	if !ok {
		return ent, false, fmt.Errorf("no serializer configured for response content type: %s", mimeType)
	}

	if err := ser.Unmarshal(responseBody, dtoPtr); err != nil {
		return ent, false, err
	}

	got, err := mapping.toEnt(ctx, dtoPtr)
	if err != nil {
		return ent, false, err
	}

	return got, true, nil
}

func (r RestClient[Entity, ID]) Update(ctx context.Context, ptr *Entity) error {
	if ptr == nil {
		return fmt.Errorf("nil pointer (%s) received",
			reflectkit.TypeOf[Entity]().String())
	}

	baseURL, err := r.getBaseURL(ctx)
	if err != nil {
		return err
	}

	var lookupID = r.LookupID
	if lookupID == nil {
		lookupID = extid.Lookup[ID, Entity]
	}

	ser := r.getSerializer(r.getMIMEType())
	mapping := r.getMapping()

	id, ok := lookupID(*ptr)
	if !ok {
		return fmt.Errorf("unable to find the %s in %s, try configure ResourceClient.LookupID",
			reflectkit.TypeOf[ID]().String(), reflectkit.TypeOf[Entity]().String())
	}

	pathParamID, err := r.getIDConverter().FormatID(id)
	if err != nil {
		return err
	}

	dto, err := mapping.toDTO(ctx, *ptr)
	if err != nil {
		return err
	}

	data, err := ser.Marshal(dto)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, pathkit.Join(baseURL, pathParamID), bytes.NewReader(data))
	if err != nil {
		return err
	}

	req.Header.Set(headerKeyContentType, r.getMIMEType())
	req.Header.Set(headerKeyAccept, r.getMIMEType())

	resp, err := r.httpClient().Do(req)
	if err != nil {
		return err
	}

	responseBody, err := bodyReadAll(resp.Body, DefaultBodyReadLimit)
	if err != nil {
		return err
	}

	if resp.StatusCode == http.StatusNotFound {
		return crud.ErrNotFound
	}

	if !statusOK(resp) {
		return makeClientErrUnexpectedResponse(req, resp, responseBody)
	}

	got, found, err := r.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if !found {
		return crud.ErrNotFound
	}

	*ptr = got
	return nil
}

func (r RestClient[Entity, ID]) DeleteByID(ctx context.Context, id ID) error {
	baseURL, err := r.getBaseURL(ctx)
	if err != nil {
		return err
	}

	pathParamID, err := r.getIDConverter().FormatID(id)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, pathkit.Join(baseURL, pathParamID), nil)
	if err != nil {
		return err
	}

	resp, err := r.httpClient().Do(req)
	if err != nil {
		return err
	}

	responseBody, err := bodyReadAll(resp.Body, DefaultBodyReadLimit)
	if err != nil {
		return err
	}

	if resp.StatusCode == http.StatusNotFound {
		return crud.ErrNotFound
	}

	if !statusOK(resp) {
		return makeClientErrUnexpectedResponse(req, resp, responseBody)
	}

	return nil
}

func (r RestClient[Entity, ID]) DeleteAll(ctx context.Context) error {
	baseURL, err := r.getBaseURL(ctx)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, baseURL, nil)
	if err != nil {
		return err
	}

	resp, err := r.httpClient().Do(req)
	if err != nil {
		return err
	}

	responseBody, err := bodyReadAll(resp.Body, DefaultBodyReadLimit)
	if err != nil {
		return err
	}

	if !statusOK(resp) {
		return makeClientErrUnexpectedResponse(req, resp, responseBody)
	}

	return nil
}

func (r RestClient[Entity, ID]) getIDConverter() idConverter[ID] {
	if r.IDConverter != nil {
		return r.IDConverter
	}
	return IDConverter[ID]{}
}

func statusOK(resp *http.Response) bool {
	return intWithin(resp.StatusCode, 200, 299)
}

func (r RestClient[Entity, ID]) getSerializer(mimeType string) Serializer {
	if r.Serializer != nil {
		return r.Serializer
	}
	if ser, done := r.lookupSerializer(mimeType); done {
		return ser
	}
	return DefaultSerializer.Serializer
}

func (r RestClient[Entity, ID]) lookupSerializer(mimeType string) (Serializer, bool) {
	mimeType = getMediaType(mimeType)
	for mt, ser := range DefaultSerializers {
		if getMediaType(mt) == mimeType {
			return ser, true
		}
	}
	return nil, false
}

var DefaultResourceClientHTTPClient http.Client = http.Client{
	Transport: RetryRoundTripper{
		RetryStrategy: retry.ExponentialBackoff{
			WaitTime: time.Second,
			Timeout:  time.Minute,
		},
	},
	Timeout: 25 * time.Second,
}

func (r RestClient[Entity, ID]) httpClient() *http.Client {
	return zerokit.Coalesce(r.HTTPClient, &DefaultResourceClientHTTPClient)
}

func (r RestClient[Entity, ID]) getMapping() Mapping[Entity] {
	if r.Mapping == nil {
		return passthroughMappingMode[Entity]()
	}
	return r.Mapping
}

func (r RestClient[Entity, ID]) getMIMEType() string {
	var zero string
	if r.MIMEType != zero {
		return r.MIMEType
	}
	return DefaultSerializer.MIMEType
}

func (r RestClient[Entity, ID]) getBaseURL(ctx context.Context) (string, error) {
	return pathsubst(ctx, r.BaseURL)
}

var pathResourceIdentifierRGX = regexp.MustCompile(`^:[\w[:punct:]]+$`)

func pathsubst(ctx context.Context, uri string) (string, error) {
	var (
		params        = PathParams(ctx)
		baseURL, path = pathkit.SplitBase(uri)
		pathParts     = []string{baseURL}
	)
	for _, part := range pathkit.Split(path) {
		if pathResourceIdentifierRGX.MatchString(part) {
			key := strings.TrimPrefix(part, ":")
			val, ok := params[key]
			if !ok {
				return "", fmt.Errorf("missing path param: %s", key)
			}
			part = val
		}
		pathParts = append(pathParts, part)
	}
	return pathkit.Join(pathParts...), nil
}

func intWithin(got, min, max int) bool {
	return min <= got && got <= max
}

func makeClientErrUnexpectedResponse(req *http.Request, resp *http.Response, body []byte) ClientErrUnexpectedResponse {
	return ClientErrUnexpectedResponse{
		StatusCode: resp.StatusCode,
		URL:        req.URL,
		Body:       string(body),
	}
}

type ClientErrUnexpectedResponse struct {
	StatusCode int
	Body       string
	URL        *url.URL
}

func (err ClientErrUnexpectedResponse) Error() string {
	msg := fmt.Sprintf("unexpected response received")
	if err.StatusCode != 0 {
		msg += fmt.Sprintf("\n%d %s", err.StatusCode, http.StatusText(err.StatusCode))
	}
	if err.URL != nil {
		msg += fmt.Sprintf("\nurl: %s", err.URL.String())
	}
	if err.Body != "" {
		msg += fmt.Sprintf("\n\n%s", err.Body)
	}
	return msg
}

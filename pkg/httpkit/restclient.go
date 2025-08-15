package httpkit

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"iter"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"go.llib.dev/frameless/pkg/dtokit"
	"go.llib.dev/frameless/pkg/httpkit/mediatype"
	"go.llib.dev/frameless/pkg/iokit"
	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/logger"
	"go.llib.dev/frameless/pkg/logging"
	"go.llib.dev/frameless/pkg/pathkit"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/pkg/resilience"
	"go.llib.dev/frameless/pkg/zerokit"
	"go.llib.dev/frameless/port/codec"
	"go.llib.dev/frameless/port/crud"
	"go.llib.dev/frameless/port/crud/extid"
	"go.llib.dev/testcase/pp"
)

type RESTClient[ENT, ID any] struct {
	// BaseURL [required] is the url base that the rest client will use to access the remote resource.
	BaseURL string
	// HTTPClient [optional] will be used to make the http requests from the rest client.
	//
	// default: httpkit.DefaultRestClientHTTPClient
	HTTPClient *http.Client
	// MediaType [optional] is used in the related headers such as Content-Type and Accept.
	//
	// default: httpkit.DefaultCodec.MediaType
	MediaType mediatype.MediaType
	// Mapping [optional] is used if the ENT must be mapped into a DTO type prior to serialization.
	//
	// default: ENT type is used as the DTO type.
	Mapping dtokit.Mapper[ENT]
	// Codec [optional] is used for the serialization process with DTO values.
	//
	// default: DefaultCodecs will be used to find a matching codec for the given media type.
	Codec codec.Codec
	// MediaTypeCodecs [optional] is a registry that helps choose the right codec for each media type.
	//
	// default: DefaultCodecs
	MediaTypeCodecs MediaTypeCodecs
	// IDFormatter [optional] is used to format the ID value into a string format that can be part of the request path.
	//
	// default: httpkit.IDFormatter[ID].Format
	IDFormatter func(ID) (string, error)
	// IDA [optional] is the ENT's ID accessor helper, to describe how to look up the ID field in a ENT.
	//
	// default: extid.Lookup[ID, ENT]
	IDA extid.Accessor[ENT, ID]
	// WithContext [optional] allows you to add data to the context for requests.
	// If you need to select a RESTful subresource and return it as a RestClient,
	// you can use this function to add the selected resource's path parameter
	// to the context using httpkit.WithPathParam.
	//
	// default: ignored
	WithContext func(context.Context) context.Context
	// PrefetchLimit is used when a methor requires fetching entities ahead.
	// If set to -1, then prefetch is disabled.
	//
	// default: 20
	PrefetchLimit int
	// BodyReadLimit is the read limit in bytes of how much response body is accepted from the server.
	// When set to -1, it accepts indifinitelly.
	//
	// default: DefaultBodyReadLimit
	BodyReadLimit int
	// DisableStreaming switches off the streaming behaviour in results processing,
	// meaning the entire response body of the RESTful API is loaded at once, rather than bit by bit.
	// By enabling DisableStreaming, you load everything into memory upfront and can close the response connection faster.
	//
	// However, this increases memory usage and stops you from handling a very long JSON stream.
	// In return, it could reduce the number of open connections, helping ease the server’s load.
	//
	// This is useful for situations:
	//   - where slowwer servers might feel overwhelmed with holding connections concurrently open (lik ruby's unicorn server)
	//   - when the server incorrect mistake the streaming based request processing as a slow-client attack.
	//
	// default: false
	DisableStreaming bool
}

type RestClientCodec interface {
	codec.Codec
	codec.ListDecoderMaker
}

func (r RESTClient[ENT, ID]) Create(ctx context.Context, ptr *ENT) error {
	ctx = r.withContext(ctx)

	if ptr == nil {
		return fmt.Errorf("nil pointer (%s) received",
			reflectkit.TypeOf[ENT]().String())
	}

	baseURL, err := r.getBaseURL(ctx)
	if err != nil {
		return err
	}

	mimeType := r.getMediaType()
	cod := r.getCodec(mimeType)
	mapping := r.getMapping()

	dto, err := mapping.MapToIDTO(ctx, *ptr)
	if err != nil {
		return err
	}

	data, err := cod.Marshal(dto)
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

	responseBody, err := r.bodyReadAll(resp.Body)
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

	dtoPtr := mapping.NewDTO()
	if err := cod.Unmarshal(responseBody, dtoPtr); err != nil {
		return err
	}

	got, err := mapping.MapFromDTO(ctx, dtoPtr)
	if err != nil {
		return err
	}

	*ptr = got
	return nil
}

func (r RESTClient[ENT, ID]) FindAll(ctx context.Context) iter.Seq2[ENT, error] {
	return func(yield func(ENT, error) bool) {
		ctx = r.withContext(ctx)
		var details []logging.Detail
		defer func() { logger.Debug(ctx, "find all entity with a rest client http request", details...) }()

		baseURL, err := r.getBaseURL(ctx)
		if err != nil {
			var zero ENT
			pp.PP(err)
			yield(zero, err)
			return
		}

		reqURL := pathkit.Join(baseURL, "/")
		details = append(details, logging.Field("url", reqURL))

		//mapping := r.getMapping()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
		if err != nil {
			var zero ENT
			yield(zero, err)
			return
		}

		reqMediaType := r.getMediaType()
		req.Header.Set(headerKeyContentType, reqMediaType)
		req.Header.Set(headerKeyAccept, reqMediaType)
		details = append(details, logging.Field("request content type", reqMediaType))
		details = append(details, logging.Field("request accept media type", reqMediaType))

		resp, err := r.httpClient().Do(req)
		if err != nil {
			var zero ENT
			yield(zero, err)
			return
		}

		details = append(details, logging.Field("status code", resp.StatusCode))

		mapping := r.getMapping()

		cod, respMediaType, ok := r.contentTypeBasedCodec(resp)
		if !ok {
			err := fmt.Errorf("no codec configured for response content type: %s", respMediaType)
			var zero ENT
			yield(zero, err)
			return
		}

		details = append(details, logging.Field("response content type", respMediaType))

		dm, ok := cod.(codec.ListDecoderMaker)

		if r.DisableStreaming || !ok {
			data, err := io.ReadAll(resp.Body)
			if err != nil {
				var zero ENT
				yield(zero, err)
				return
			}

			ptr := mapping.NewDTOSlice()
			if err := cod.Unmarshal(data, ptr); err != nil {
				var zero ENT
				yield(zero, err)
				return
			}

			got, err := mapping.MapFromDTOSlice(ctx, ptr)
			if err != nil {
				var zero ENT
				yield(zero, fmt.Errorf("error while mapping from DTO: %w", err))
				return
			}

			for _, v := range got {
				if !yield(v, nil) {
					return
				}
			}
			return
		}

		dec := dm.MakeListDecoder(resp.Body)
		for dec.Next() {
			ptr := mapping.NewDTO()
			if err := dec.Decode(ptr); err != nil {
				var zero ENT
				if !yield(zero, err) {
					return
				}
				continue
			}
			if !yield(mapping.MapFromDTO(ctx, ptr)) {
				return
			}
		}
		if err := dec.Err(); err != nil {
			var zero ENT
			if !yield(zero, err) {
				return
			}
		}
	}
}

func (r RESTClient[ENT, ID]) FindByID(ctx context.Context, id ID) (ent ENT, found bool, err error) {
	ctx = r.withContext(ctx)

	var details []logging.Detail
	defer func() {
		details = append(details, logging.Field("found", found))
		if err != nil {
			details = append(details, logging.ErrField(err))
		}
		logger.Debug(ctx, "find entity by id with a rest client http request", details...)
	}()

	details = append(details,
		logging.Field("entity type", reflectkit.TypeOf[ENT]().String()),
		logging.Field("id", id),
	)

	mapping := r.getMapping()

	pathParamID, err := r.formatID(id)
	if err != nil {
		return ent, false, err
	}

	baseURL, err := r.getBaseURL(ctx)
	if err != nil {
		return ent, false, err
	}

	requestURL := pathkit.Join(baseURL, pathParamID)

	details = append(details, logging.Field("url", requestURL))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return ent, false, err
	}

	requestMediaType := r.getMediaType()
	req.Header.Set(headerKeyContentType, requestMediaType)
	req.Header.Set(headerKeyAccept, requestMediaType)
	details = append(details, logging.Field("request content type", requestMediaType))
	details = append(details, logging.Field("request accept media type", requestMediaType))

	resp, err := r.httpClient().Do(req)
	if err != nil {
		return ent, false, err
	}

	details = append(details, logging.Field("status code", resp.StatusCode))

	responseBody, err := r.bodyReadAll(resp.Body)
	if err != nil {
		return ent, false, err
	}

	details = append(details, logging.Field("response body", string(responseBody)))

	if resp.StatusCode == http.StatusNotFound {
		return ent, false, nil
	}

	if !statusOK(resp) {
		return ent, false, makeClientErrUnexpectedResponse(req, resp, responseBody)
	}

	cdk, responseMediaType, ok := r.contentTypeBasedCodec(resp)
	if !ok {
		return ent, false, fmt.Errorf("no codec configured for response content type: %s", responseMediaType)
	}

	details = append(details, logging.Field("response content type", responseMediaType))

	dtoPtr := mapping.NewDTO()
	if err := cdk.Unmarshal(responseBody, dtoPtr); err != nil {
		return ent, false, err
	}

	got, err := mapping.MapFromDTO(ctx, dtoPtr)
	if err != nil {
		return ent, false, err
	}

	return got, true, nil
}

func (r RESTClient[ENT, ID]) FindByIDs(ctx context.Context, ids ...ID) iter.Seq2[ENT, error] {
	return func(yield func(ENT, error) bool) {
		if err := ctx.Err(); err != nil {
			var zero ENT
			yield(zero, err)
			return
		}

		var itr iterkit.SeqE[ENT] = func(yield func(ENT, error) bool) {
			var zero ENT
			for _, id := range ids {
				if err := ctx.Err(); err != nil {
					yield(zero, err)
					return
				}
				ent, found, err := r.FindByID(ctx, id)
				if err != nil {
					yield(zero, err)
					return
				}
				if !found {
					yield(zero, fmt.Errorf("%w: id=%v", crud.ErrNotFound, id))
					return
				}
				if !yield(ent, nil) {
					return
				}
			}
		}

		next, stop := iter.Pull2(itr)
		var prefectDone bool
		defer func() {
			if prefectDone {
				return
			}
			stop()
		}()

		var prefetchedEntities []ENT
		limit := r.getPrefetchLimit()
		for i := 0; i < limit; i++ {
			ent, err, ok := next()
			if !ok {
				break
			}
			if err != nil {
				var zero ENT
				yield(zero, err)
				return
			}
			prefetchedEntities = append(prefetchedEntities, ent)
		}

		prefectDone = true

		src := iterkit.Merge2(
			iterkit.AsSeqE(iterkit.FromSlice(prefetchedEntities)),
			iterkit.FromPull2(next, stop),
		)

		for v, err := range src {
			if !yield(v, err) {
				return
			}
		}
	}
}

func (r RESTClient[ENT, ID]) Update(ctx context.Context, ptr *ENT) error {
	ctx = r.withContext(ctx)

	if ptr == nil {
		return fmt.Errorf("nil pointer (%s) received",
			reflectkit.TypeOf[ENT]().String())
	}

	baseURL, err := r.getBaseURL(ctx)
	if err != nil {
		return err
	}

	var idAccessor = r.IDA

	ser := r.getCodec(r.getMediaType())
	mapping := r.getMapping()

	id, ok := idAccessor.Lookup(*ptr)
	if !ok {
		return fmt.Errorf("unable to find the %s in %s, try configure RESTClient.IDA",
			reflectkit.TypeOf[ID]().String(), reflectkit.TypeOf[ENT]().String())
	}

	pathParamID, err := r.formatID(id)
	if err != nil {
		return err
	}

	dto, err := mapping.MapToIDTO(ctx, *ptr)
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

	req.Header.Set(headerKeyContentType, r.getMediaType())
	req.Header.Set(headerKeyAccept, r.getMediaType())

	resp, err := r.httpClient().Do(req)
	if err != nil {
		return err
	}

	responseBody, err := r.bodyReadAll(resp.Body)
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

func (r RESTClient[ENT, ID]) DeleteByID(ctx context.Context, id ID) error {
	ctx = r.withContext(ctx)

	baseURL, err := r.getBaseURL(ctx)
	if err != nil {
		return err
	}

	pathParamID, err := r.formatID(id)
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

	responseBody, err := r.bodyReadAll(resp.Body)
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

func (r RESTClient[ENT, ID]) DeleteAll(ctx context.Context) error {
	ctx = r.withContext(ctx)

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

	responseBody, err := bodyReadAll(resp.Body, r.BodyReadLimit)
	if err != nil {
		return err
	}

	if !statusOK(resp) {
		return makeClientErrUnexpectedResponse(req, resp, responseBody)
	}

	return nil
}

func (r RESTClient[ENT, ID]) formatID(id ID) (string, error) {
	if r.IDFormatter != nil {
		return r.IDFormatter(id)
	}
	return IDConverter[ID]{}.FormatID(id)
}

func statusOK(resp *http.Response) bool {
	return intWithin(resp.StatusCode, 200, 299)
}

func (r RESTClient[ENT, ID]) getCodec(mimeType string) codec.Codec {
	if c, ok := r.MediaTypeCodecs.Lookup(mimeType); ok {
		return c
	}
	if r.Codec != nil {
		return r.Codec
	}
	return defaultCodec.Codec
}

func (r RESTClient[ENT, ID]) contentTypeBasedCodec(resp *http.Response) (codec.Codec, mediatype.MediaType, bool) {
	mt := string(resp.Header.Get(headerKeyContentType))
	c, ok := r.MediaTypeCodecs.Lookup(mt)
	if !ok && r.Codec != nil {
		c, ok = r.Codec, true
	}
	return c, mt, ok
}

var DefaultRestClientHTTPClient http.Client = http.Client{
	Transport: RetryRoundTripper{
		RetryStrategy: resilience.ExponentialBackoff{
			Delay:   time.Second,
			Timeout: time.Minute,
		},
	},
	Timeout: 25 * time.Second,
}

func (r RESTClient[ENT, ID]) httpClient() *http.Client {
	return zerokit.Coalesce(r.HTTPClient, &DefaultRestClientHTTPClient)
}

func (r RESTClient[ENT, ID]) getMapping() dtokit.Mapper[ENT] {
	if r.Mapping == nil {
		return passthroughMappingMode[ENT]()
	}
	return r.Mapping
}

func (r RESTClient[ENT, ID]) getPrefetchLimit() int {
	if 0 < r.PrefetchLimit {
		return r.PrefetchLimit
	}
	if r.PrefetchLimit < 0 {
		return 0
	}
	return 20 // default
}

func (r RESTClient[ENT, ID]) getMediaType() string {
	var zero string
	if r.MediaType != zero {
		return r.MediaType
	}
	return defaultCodec.MediaType
}

func (r RESTClient[ENT, ID]) getBaseURL(ctx context.Context) (string, error) {
	return pathsubst(ctx, r.BaseURL)
}

func (r RESTClient[ENT, ID]) withContext(ctx context.Context) context.Context {
	if r.WithContext != nil {
		return r.WithContext(ctx)
	}
	return ctx
}

func (r RESTClient[ENT, ID]) bodyReadAll(body io.ReadCloser) ([]byte, error) {
	data, err := bodyReadAll(body, r.BodyReadLimit)
	if errors.Is(err, iokit.ErrReadLimitReached) {
		return nil, ErrResponseEntityTooLarge
	}
	return data, nil
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

		RequestMethod: req.Method,
		RequestURL:    req.URL,
	}
}

type ClientErrUnexpectedResponse struct {
	StatusCode int
	Body       string
	URL        *url.URL

	RequestMethod string
	RequestURL    *url.URL
}

func (err ClientErrUnexpectedResponse) Error() string {
	msg := "unexpected response received"
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

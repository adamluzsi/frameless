package httpkit

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"time"

	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/httpkit/internal"
	"go.llib.dev/frameless/pkg/iokit"
	"go.llib.dev/frameless/pkg/logger"
	"go.llib.dev/frameless/pkg/logging"
	"go.llib.dev/frameless/pkg/pathkit"
	"go.llib.dev/frameless/pkg/resilience"
	"go.llib.dev/testcase/clock"
)

type RoundTripperFunc func(request *http.Request) (*http.Response, error)

func (fn RoundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

type RetryRoundTripper struct {
	// Transport specifies the mechanism by which individual
	// HTTP requests are made.
	//
	// Default: http.DefaultTransport
	Transport http.RoundTripper
	// RetryStrategy will be used to evaluate if a new retry attempt should be done.
	//
	// Default: retry.ExponentialBackoff
	RetryStrategy resilience.RetryPolicy[resilience.FailureCount]
	// OnStatus is an [OPTIONAL] configuration field that could contain whether a certain http status code should be retried or not.
	// The RetryRoundTripper has a default behaviour about which status code can be retried, and this option can override that.
	OnStatus map[int]bool
}

var temporaryErrorResponseCodes = map[int]struct{}{
	http.StatusInternalServerError: {},
	http.StatusBadGateway:          {},
	http.StatusGatewayTimeout:      {},
	http.StatusServiceUnavailable:  {},
	http.StatusInsufficientStorage: {},
	http.StatusTooManyRequests:     {},
	http.StatusRequestTimeout:      {},
}

// RoundTrip
//
// TODO: optional waiting based on the Retry-After header
func (rt RetryRoundTripper) RoundTrip(request *http.Request) (resp *http.Response, err error) {
	rs := rt.getRetryStrategy()
	body, err := rt.readBody(request)
	if err != nil {
		return nil, err
	}
	request.Body = io.NopCloser(body)

	for i := 0; rs.ShouldTry(request.Context(), i); i++ {
		// reset body to original state before making the request
		if _, err := body.Seek(0, io.SeekStart); err != nil {
			return nil, err
		}

		resp, err = rt.transport().RoundTrip(request)

		if err != nil {
			if rt.isRetriableError(err) {
				continue
			}
			return resp, err
		}

		if rt.isRetriableStatus(resp.StatusCode) {
			continue
		}

		return resp, nil
	}
	if err := request.Context().Err(); err != nil {
		return nil, err
	}
	return
}

func (rt RetryRoundTripper) isRetriableStatus(code int) bool {
	if rt.OnStatus != nil {
		if should, ok := rt.OnStatus[code]; ok {
			return should
		}
	}
	_, ok := temporaryErrorResponseCodes[code]
	return ok
}

func (rt RetryRoundTripper) transport() http.RoundTripper {
	if rt.Transport == nil {
		return http.DefaultTransport
	}
	return rt.Transport
}

func (rt RetryRoundTripper) readBody(req *http.Request) (io.ReadSeeker, error) {
	reqBody := req.Body
	if reqBody == nil {
		reqBody = io.NopCloser(bytes.NewReader([]byte{}))
	}
	bs, err := io.ReadAll(reqBody)
	err = errorkit.Merge(err, reqBody.Close())
	if err != nil {
		return nil, err
	}
	return iokit.NewBuffer(bs), err
}

func (rt RetryRoundTripper) isRetriableError(err error) bool {
	return errors.Is(err, http.ErrHandlerTimeout) ||
		errors.Is(err, net.ErrClosed) ||
		isTimeout(err)
}

func (rt RetryRoundTripper) getRetryStrategy() resilience.RetryPolicy[resilience.FailureCount] {
	if rt.RetryStrategy != nil {
		return rt.RetryStrategy
	}
	return &resilience.ExponentialBackoff{}
}

func isTimeout(err error) bool {
	type errorWithTimeoutInfo interface {
		error
		Timeout() bool
	}
	if v, ok := err.(errorWithTimeoutInfo); ok && v.Timeout() {
		return true
	}
	return false
}

// WithAccessLog is a MiddlewareFactoryFunc for adding AccessLog to a http.Handler middleware stack.
func WithAccessLog(next http.Handler) http.Handler {
	return AccessLog{Next: next}
}

type AccessLog struct {
	Next http.Handler

	AdditionalLoggingDetail func(w http.ResponseWriter, r *http.Request) logging.Detail
}

func (mw AccessLog) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body := &requestBodyAccessLog{Body: r.Body}
	r.Body = body
	defer mw.doLog(w, r, clock.Now(), body)
	mw.Next.ServeHTTP(w, r)
}

func (mw AccessLog) doLog(w http.ResponseWriter, r *http.Request, startTime time.Time, body *requestBodyAccessLog) {
	endTime := clock.Now()
	info := getResponseInfo(w)
	dur := endTime.Sub(startTime)
	fields := logging.Fields{
		"method":               r.Method,
		"path":                 r.URL.Path,
		"query":                r.URL.RawQuery,
		"duration":             dur.String(),
		"duration_ms":          dur.Milliseconds(),
		"remote_address":       r.RemoteAddr,
		"host":                 r.Host,
		"status":               info.StatusCode,
		"request_body_length":  body.Length,
		"response_body_length": int(info.Written),
	}
	var lds = []logging.Detail{fields}
	if mw.AdditionalLoggingDetail != nil {
		if ld := mw.AdditionalLoggingDetail(w, r); ld != nil {
			lds = append(lds, ld)
		}
	}
	logger.Info(r.Context(), "http-access-log", lds...)
}

type requestBodyAccessLog struct {
	Body   io.ReadCloser
	Length int
}

func (r *requestBodyAccessLog) Read(p []byte) (n int, err error) {
	n, err = r.Body.Read(p)
	r.Length += n
	return n, err
}

func (r *requestBodyAccessLog) Close() error {
	return r.Body.Close()
}

type responseInfo struct {
	StatusCode int
	Written    int64
}

// getResponseInfo uses reflection to get the status code from the response writer
// because otherwise it would be a huge pain to maintain passthrough support for:
// - http.Pusher
// - http.Flusher
// - http.Hijacker
// - http.CloseNotifier
// - etc
//
// This implementation specifically aims to extract data from http.response
func getResponseInfo(rw http.ResponseWriter) responseInfo {
	var info responseInfo
	visitForStatusCode(&info, reflect.ValueOf(rw), map[reflect.Value]struct{}{})
	return info
}

func visitForStatusCode(info *responseInfo, rv reflect.Value, recursionGuard map[reflect.Value]struct{}) {
	if _, ok := recursionGuard[rv]; ok {
		return
	}
	recursionGuard[rv] = struct{}{}
	defer func() { delete(recursionGuard, rv) }()

	switch rv.Kind() {
	case reflect.Struct:
		for i, numField := 0, rv.NumField(); i < numField; i++ {
			field := rv.Field(i)
			switch {
			case rv.Type().Field(i).Name == "status" && field.Kind() == reflect.Int:
				info.StatusCode = int(field.Int())
			case rv.Type().Field(i).Name == "written" && field.Kind() == reflect.Int64:
				info.Written = field.Int()
			default:
				visitForStatusCode(info, field, recursionGuard)
			}
		}
	case reflect.Pointer:
		visitForStatusCode(info, rv.Elem(), recursionGuard)
	default:
	}
}

// Mount will help to register a handler on a request multiplexer in both as the concrete path to the handler and as a prefix match.
// example:
//
//	if pattern -> "/something"
//	registered as "/something" for exact match
//	registered as "/something/" for prefix match
func Mount(mux Multiplexer, pattern string, handler http.Handler) {
	pattern = pathkit.Clean(pattern)
	handler = MountPoint(pattern, handler)
	handler = http.StripPrefix(pattern, handler)
	mux.Handle(pattern, handler)
	mux.Handle(pattern+`/`, handler)
}

func MountPoint(mountPoint string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r, rc := internal.WithRoutingContext(r)
		rc.Travel(mountPoint)
		next.ServeHTTP(w, r)
	})
}

// Multiplexer represents a http request Multiplexer.
type Multiplexer interface {
	Handle(pattern string, handler http.Handler)
}

// MiddlewareFactoryFunc is a constructor function that is meant to wrap an http.Handler with given middleware.
// Its http.Handler argument represents the next middleware http.Handler in the pipeline.
type MiddlewareFactoryFunc func(next http.Handler) http.Handler

// WithMiddleware will combine an http.Handler with a stack of middleware factory functions.
// The order in which you pass the MiddlewareFactoryFunc -s is the same as the order,
// they will be called during the http.Handler.ServeHTTP method call.
func WithMiddleware(handler http.Handler, ffns ...MiddlewareFactoryFunc) http.Handler {
	for i := len(ffns) - 1; 0 <= i; i-- {
		if ffns[i] == nil {
			continue
		}
		handler = ffns[i](handler)
	}
	return handler
}

// RoundTripperFactoryFunc is a constructor function that is meant to wrap an http.RoundTripper with given middleware.
// Its http.RoundTripper argument represents the next middleware http.RoundTripper in the pipeline.
type RoundTripperFactoryFunc func(next http.RoundTripper) http.RoundTripper

// WithRoundTripper will combine an http.RoundTripper with a stack of middleware factory functions.
// The execution order is in which you pass the factory funcs.
func WithRoundTripper(transport http.RoundTripper, rts ...RoundTripperFactoryFunc) http.RoundTripper {
	if transport == nil {
		transport = http.DefaultTransport
	}
	for i := len(rts) - 1; 0 <= i; i-- {
		if rts[i] == nil {
			continue
		}
		transport = rts[i](transport)
	}
	return transport
}

type ErrorHandler interface {
	HandleError(w http.ResponseWriter, r *http.Request, err error)
}

// LookupRequest will check if the context contains an http request.
// LookupRequest is mostly meant to be used from functions like Index in RestResource.
func LookupRequest(ctx context.Context) (*http.Request, bool) {
	return internal.LookupRequest(ctx)
}

func stripPrefix(prefix string, h http.Handler) http.Handler {
	if prefix == "" {
		return h
	}
	if u, err := url.ParseRequestURI(prefix); err == nil {
		prefix = u.RequestURI()
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		epath := r.URL.RequestURI()
		if !strings.HasPrefix(epath, prefix) {
			http.NotFound(w, r)
			return
		}

		epath = pathkit.Canonical(strings.TrimPrefix(epath, prefix))

		uri, err := url.ParseRequestURI(epath)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		nURL := new(url.URL)
		*nURL = *r.URL
		nURL.Path = uri.Path
		nURL.RawPath = uri.RawPath

		nr := new(http.Request)
		*nr = *r
		nr.URL = nURL

		h.ServeHTTP(w, nr)
	})
}

// IsSuccess: Success - The action was successfully received, understood, and accepted
func IsSuccess[V int | *http.Response](v V) bool {
	switch v := any(v).(type) {
	case *http.Response:
		if v == nil {
			return false
		}
		return IsSuccess(v.StatusCode)
	case int:
		return 200 <= v && v < 300
	default:
		panic("not-implemented")
	}
}

// 1xx: Informational - Request received, continuing process
// 3xx: Redirection - Further action must be taken in order to complete the request
// 4xx: Client Error - The request contains bad syntax or cannot be fulfilled
// 5xx: Server Error - The server failed to fulfill an apparently valid request

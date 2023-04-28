package httputil

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/adamluzsi/frameless/pkg/buffers"
	"github.com/adamluzsi/frameless/pkg/errorutil"
	"github.com/adamluzsi/frameless/pkg/logger"
	"github.com/adamluzsi/frameless/pkg/retry"
	"github.com/adamluzsi/testcase/clock"
	"io"
	"net"
	"net/http"
	"reflect"
	"time"
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
	RetryStrategy retry.Strategy
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
		if _, err := body.Seek(io.SeekStart, 0); err != nil {
			return nil, err
		}

		resp, err = rt.transport().RoundTrip(request)

		if err != nil {
			if rt.isRetriableError(err) {
				continue
			}
			return resp, err
		}

		if _, ok := temporaryErrorResponseCodes[resp.StatusCode]; ok {
			continue
		}

		return resp, nil
	}
	return
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
	err = errorutil.Merge(err, reqBody.Close())
	if err != nil {
		return nil, err
	}
	return buffers.New(bs), err
}

func (rt RetryRoundTripper) isRetriableError(err error) bool {
	return errors.Is(err, http.ErrHandlerTimeout) ||
		errors.Is(err, net.ErrClosed) ||
		isTimeout(err)
}

func (rt RetryRoundTripper) getRetryStrategy() retry.Strategy {
	if rt.RetryStrategy != nil {
		return rt.RetryStrategy
	}
	return &retry.ExponentialBackoff{}
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

type AccessLog struct {
	Next http.Handler

	AdditionalLoggingDetail func(w http.ResponseWriter, r *http.Request) logger.LoggingDetail
}

func (mw AccessLog) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body := &requestBodyAccessLog{Body: r.Body}
	r.Body = body
	defer mw.doLog(w, r, clock.TimeNow(), body)
	mw.Next.ServeHTTP(w, r)
}

func (mw AccessLog) doLog(w http.ResponseWriter, r *http.Request, startTime time.Time, body *requestBodyAccessLog) {
	endTime := clock.TimeNow()
	info := getResponseInfo(w)
	fields := logger.Fields{
		"method":               r.Method,
		"path":                 r.URL.Path,
		"query":                r.URL.RawQuery,
		"duration":             mw.fmtDuration(endTime.Sub(startTime)) + "s",
		"remote_address":       r.RemoteAddr,
		"host":                 r.Host,
		"status":               info.StatusCode,
		"request_body_length":  body.Length,
		"response_body_length": int(info.Written),
	}
	var lds = []logger.LoggingDetail{fields}
	if mw.AdditionalLoggingDetail != nil {
		if ld := mw.AdditionalLoggingDetail(w, r); ld != nil {
			lds = append(lds, ld)
		}
	}
	logger.Info(r.Context(), "http-access-log", lds...)
}

func (mw AccessLog) fmtDuration(duration time.Duration) string {
	durationFloat := float64(duration) / float64(time.Second)
	return fmt.Sprintf("%.3f", durationFloat)
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
	case reflect.Ptr:
		visitForStatusCode(info, rv.Elem(), recursionGuard)
	default:
	}
}

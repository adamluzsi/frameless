package resilience

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"

	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/iokit"
)

// HTTPRoundTripper is an http.RoundTripper that adds automatic retry
// logic to HTTP requests. It wraps an underlying transport and retries
// requests when recoverable errors occur (e.g., network timeouts,
// 5xx status codes) based on a configurable retry strategy.
type HTTPRoundTripper struct {
	// Transport specifies the mechanism by which individual
	// HTTP requests are made.
	//
	// Default: http.DefaultTransport
	Transport http.RoundTripper
	// RetryStrategy [OPTIONAL] will be used to evaluate if a new retry attempt should be done.
	//
	// Default: DefaultRetryStrategy
	RetryStrategy RetryStrategy
	// OnStatus [OPTIONAL] is a configuration field
	// that describes whether a certain http status code should be retried or not.
	//
	// This field allows the overwriting the default behaviour
	// about which status code can be retried.
	OnStatus map[int]bool
	// DisableResponseBodyRetry will disable http response body repeating upon error occurrence.
	// with http.Response#Body reading.
	// Else the resilience HTTPRoundTripper will retry body read issues,
	// for http Methods which are not meant to mutate state on the server side, such as GET.
	DisableResponseBodyRetry bool
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
func (rt HTTPRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	body, err := rt.readBody(request)
	if err != nil {
		return nil, err
	}
	request.Body = io.NopCloser(body)
	var transport = rt.transport()

	var mkRequest = func() (*http.Request, error) {
		// reset body to original state before making the request
		if _, err := body.Seek(0, io.SeekStart); err != nil {
			return nil, err
		}
		return request, nil
	}

	ctx := request.Context()

	response, err := rt.tryRoundTrip(ctx, transport, mkRequest)
	if err != nil {
		return response, err
	}

	if !rt.DisableResponseBodyRetry && isHTTPMethodRepeatable(request.Method) && response.Body != nil {
		var body = response.Body
		response.Body = &Reader{
			reader:        body,
			Context:       request.Context(),
			RetryStrategy: rt.RetryStrategy,
			Open: func() (io.Reader, error) {
				resp, err := rt.tryRoundTrip(ctx, transport, mkRequest)
				if err != nil {
					return nil, err
				}
				*response = *resp
				if resp == nil {
					return nil, fmt.Errorf("nil response received from http round trip")
				}
				return resp.Body, nil
			},
		}
	}

	return response, err
}

func isHTTPMethodRepeatable(method string) bool {
	_, ok := repeatableHTTPMethods[method]
	return ok
}

var repeatableHTTPMethods = map[string]struct{}{
	http.MethodGet:     {},
	http.MethodHead:    {},
	http.MethodConnect: {},
	http.MethodOptions: {},
	http.MethodTrace:   {},
}

func (rt HTTPRoundTripper) tryRoundTrip(ctx context.Context, transport http.RoundTripper, mkRequest func() (*http.Request, error)) (*http.Response, error) {
	var (
		response *http.Response
		err      error
	)
	for range Retries(ctx, rt.RetryStrategy) {
		var request *http.Request

		request, err = mkRequest()
		if err != nil {
			continue
		}

		response, err = transport.RoundTrip(request)

		// retry due to error
		if err != nil {
			if rt.isRecoverableError(err) {
				continue
			}
			return response, err
		}

		// retry due to status code
		if rt.isRecoverableStatus(response.StatusCode) {
			continue
		}

		return response, nil
	}
	if err != nil {
		return response, err
	}
	if err := ctx.Err(); err != nil {
		return response, err
	}
	return response, err
}

func (rt HTTPRoundTripper) isRecoverableStatus(code int) bool {
	if rt.OnStatus != nil {
		if should, ok := rt.OnStatus[code]; ok {
			return should
		}
	}
	_, ok := temporaryErrorResponseCodes[code]
	return ok
}

func (rt HTTPRoundTripper) transport() http.RoundTripper {
	if rt.Transport == nil {
		return http.DefaultTransport
	}
	return rt.Transport
}

func (rt HTTPRoundTripper) readBody(req *http.Request) (io.ReadSeeker, error) {
	reqBody := req.Body
	if reqBody == nil {
		reqBody = io.NopCloser(bytes.NewReader([]byte{}))
	}
	if body, ok := reqBody.(io.ReadSeeker); ok {
		return body, nil
	}
	data, err := io.ReadAll(reqBody)
	err = errorkit.Merge(err, reqBody.Close())
	if err != nil {
		return nil, err
	}
	return iokit.NewBuffer(data), err
}

func (rt HTTPRoundTripper) isRecoverableError(err error) bool {
	return errors.Is(err, http.ErrHandlerTimeout) ||
		errors.Is(err, net.ErrClosed) ||
		rt.isTimeout(err)
}

func (rt HTTPRoundTripper) isTimeout(err error) bool {
	type errorWithTimeoutInfo interface {
		error
		Timeout() bool
	}
	if v, ok := err.(errorWithTimeoutInfo); ok && v.Timeout() {
		return true
	}
	return false
}

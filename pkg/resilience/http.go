package resilience

import (
	"bytes"
	"errors"
	"io"
	"net"
	"net/http"

	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/iokit"
)

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
func (rt HTTPRoundTripper) RoundTrip(request *http.Request) (resp *http.Response, err error) {
	rs := GetRetryStrategy(rt.RetryStrategy)
	body, err := rt.readBody(request)
	if err != nil {
		return nil, err
	}
	request.Body = io.NopCloser(body)

	for range rs.Retry(request.Context()) {
		// reset body to original state before making the request
		if _, err := body.Seek(0, io.SeekStart); err != nil {
			return nil, err
		}

		resp, err = rt.transport().RoundTrip(request)

		if err != nil {
			if rt.isRecoverableError(err) {
				continue
			}
			return resp, err
		}

		if rt.isRecoverableStatus(resp.StatusCode) {
			continue
		}

		return resp, nil
	}
	if err := request.Context().Err(); err != nil {
		return nil, err
	}
	return
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
	bs, err := io.ReadAll(reqBody)
	err = errorkit.Merge(err, reqBody.Close())
	if err != nil {
		return nil, err
	}
	return iokit.NewBuffer(bs), err
}

func (rt HTTPRoundTripper) isRecoverableError(err error) bool {
	return errors.Is(err, http.ErrHandlerTimeout) ||
		errors.Is(err, net.ErrClosed) ||
		rt.isTimeout(err)
}

func (rt HTTPRoundTripper) getRetryStrategy() RetryStrategy {
	if rt.RetryStrategy != nil {
		return rt.RetryStrategy
	}
	return DefaultRetryStrategy
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

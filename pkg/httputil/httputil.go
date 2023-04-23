package httputil

import (
	"bytes"
	"errors"
	"github.com/adamluzsi/frameless/pkg/errorutil"
	"io"
	"net"
	"net/http"
)

type RoundTripperFunc func(request *http.Request) (*http.Response, error)

func (fn RoundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

type RetryRoundTripper struct {
	// Transport specifies the mechanism by which individual
	// HTTP requests are made.
	// If nil, DefaultTransport is used.
	Transport http.RoundTripper

	// Strategy
}

var temporaryErrorResponseCodes = map[int]struct{}{
	http.StatusInternalServerError: {},
	http.StatusBadGateway:          {},
	http.StatusServiceUnavailable:  {},
	http.StatusGatewayTimeout:      {},
	http.StatusInsufficientStorage: {},
}

func (rt RetryRoundTripper) RoundTrip(request *http.Request) (resp *http.Response, err error) {
	bs, err := rt.readBody(request)
	if err != nil {
		return nil, err
	}

	const max = 10
	for i, m := 0, max-1; i < m; i++ {
		// reset body to original state before making the request
		request.Body = io.NopCloser(bytes.NewReader(bs))

		resp, err = rt.transport().RoundTrip(request)
		if i == max {
			return resp, err
		}

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

func (rt RetryRoundTripper) readBody(req *http.Request) ([]byte, error) {
	reqBody := req.Body
	if reqBody == nil {
		reqBody = io.NopCloser(bytes.NewReader([]byte{}))
	}
	bs, err := io.ReadAll(reqBody)
	err = errorutil.Merge(err, reqBody.Close())
	return bs, err
}

func (rt RetryRoundTripper) isRetriableError(err error) bool {
	if errors.Is(err, http.ErrHandlerTimeout) || errors.Is(err, net.ErrClosed) {
		return true
	}
	var timeout bool
	var temporary bool

	if v, ok := err.(interface{ Timeout() bool }); ok {
		timeout = v.Timeout()
	}

	return temporary || timeout
}

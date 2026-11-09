package resilience

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/iokit"
	"go.llib.dev/frameless/pkg/zerokit"
	"go.llib.dev/testcase/clock"
)

// HTTPRoundTripper is an http.RoundTripper that adds automatic retry
// logic to HTTP requests. It wraps an underlying transport and retries
// requests when recoverable errors occur (e.g., network timeouts,
// 5xx status codes) based on a configurable retry strategy.
//
// When a retryable response (e.g. 429 Too Many Requests or 503 Service Unavailable)
// tells how long the client should back off, the next attempt is delayed at least until then.
// The supported retry hints, from the highest to the lowest precedence, are:
//
//   - Retry-After-Ms and X-Ms-Retry-After-Ms, as milliseconds (Azure)
//   - Retry-After, as delay seconds or an HTTP-date (RFC 9110)
//   - RateLimit, as the "t" parameter of a policy with no remaining quota ("r=0"),
//     following draft-ietf-httpapi-ratelimit-headers
//   - RateLimit-Reset and X-RateLimit-Reset, when the matching *-Remaining header is absent or zero.
//     The value can be delay seconds, a Unix epoch timestamp, an HTTP-date or an RFC 3339 timestamp.
//
// Absolute times are measured relative to the response's Date header,
// so a skewed local clock doesn't distort the delay.
// The hint is a minimum delay: if the RetryStrategy waits longer, its delay wins.
// See MaxRetryAfter for what happens when the server asks for too long a delay.
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
	// MaxRetryAfter [OPTIONAL] is the longest delay that HTTPRoundTripper is willing to wait
	// when a retryable response asks for one with a retry hint, such as the Retry-After header.
	// When the server asks for a longer delay, or for one that outlasts the request context's deadline,
	// the response is returned as it is, instead of retrying sooner than the server asked for.
	// A negative value disables retry hints, leaving the delay between attempts to the RetryStrategy alone.
	//
	// Default: 1 minute
	MaxRetryAfter time.Duration
}

const defaultMaxRetryAfter = time.Minute

var temporaryErrorResponseCodes = map[int]struct{}{
	http.StatusInternalServerError: {},
	http.StatusBadGateway:          {},
	http.StatusGatewayTimeout:      {},
	http.StatusServiceUnavailable:  {},
	http.StatusInsufficientStorage: {},
	http.StatusTooManyRequests:     {},
	http.StatusRequestTimeout:      {},
}

// RoundTrip implements http.RoundTripper.
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
		// notBefore is the earliest time the server allows the next attempt.
		notBefore time.Time
	)
	for range Retries(ctx, rt.RetryStrategy) {
		if !waitUntil(ctx, notBefore) {
			break
		}

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
			delay, ok := rt.retryAfter(response)
			if !ok || delay <= 0 {
				continue
			}
			if !rt.canWait(ctx, delay) {
				// Retrying sooner than the server asked for would only be rejected again,
				// so the caller gets the response and can decide what to do with it.
				return response, nil
			}
			notBefore = clock.Now().Add(delay)
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

func (rt HTTPRoundTripper) retryAfter(response *http.Response) (time.Duration, bool) {
	if rt.getMaxRetryAfter() < 0 {
		return 0, false
	}
	return retryAfter(response)
}

func (rt HTTPRoundTripper) canWait(ctx context.Context, delay time.Duration) bool {
	if rt.getMaxRetryAfter() < delay {
		return false
	}
	// Context deadlines are set with the wall clock (e.g. by http.Client#Timeout),
	// so they are compared against the wall clock as well.
	if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) < delay {
		return false
	}
	return true
}

func (rt HTTPRoundTripper) getMaxRetryAfter() time.Duration {
	return zerokit.Coalesce(rt.MaxRetryAfter, defaultMaxRetryAfter)
}

// waitUntil blocks until the given time is reached.
// It reports false if the context is done before that.
func waitUntil(ctx context.Context, t time.Time) bool {
	if t.IsZero() {
		return ctx.Err() == nil
	}
	d := t.Sub(clock.Now())
	if d <= 0 {
		return ctx.Err() == nil
	}
	select {
	case <-ctx.Done():
		return false
	case <-clock.After(d):
		return true
	}
}

// retryAfter tells how long the server asks the client to wait before retrying,
// based on the response's retry hint headers.
// A zero or negative delay means that the client may retry right away.
func retryAfter(response *http.Response) (time.Duration, bool) {
	if response == nil || response.Header == nil {
		return 0, false
	}
	var (
		header = response.Header
		now    = responseTime(response)
	)
	for _, key := range []string{"Retry-After-Ms", "X-Ms-Retry-After-Ms"} {
		if d, ok := parseDelay(header.Get(key), time.Millisecond); ok {
			return d, true
		}
	}
	if value := header.Get("Retry-After"); value != "" {
		if d, ok := parseDelay(value, time.Second); ok {
			return d, true
		}
		if t, err := http.ParseTime(value); err == nil {
			return t.Sub(now), true
		}
	}
	// RFC 9110 Retry-After takes precedence over the RateLimit fields,
	// as required by draft-ietf-httpapi-ratelimit-headers.
	if d, ok := parseRateLimit(header.Values("RateLimit")); ok {
		return d, true
	}
	for _, prefix := range []string{"RateLimit", "X-RateLimit"} {
		if d, ok := parseRateLimitReset(header, prefix, now); ok {
			return d, true
		}
	}
	return 0, false
}

// responseTime is the time when the server generated the response.
// Measuring absolute retry hints from it avoids clock skew between the client and the server.
func responseTime(response *http.Response) time.Time {
	if date, err := http.ParseTime(response.Header.Get("Date")); err == nil {
		return date
	}
	return clock.Now()
}

// parseDelay parses a non-negative number, which may be fractional, as a multiple of the given unit.
func parseDelay(value string, unit time.Duration) (time.Duration, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	n, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsNaN(n) || math.IsInf(n, 0) || n < 0 {
		return 0, false
	}
	return toDuration(n, unit), true
}

// toDuration converts a non-negative amount of units into a time.Duration,
// capping it at the longest representable duration instead of overflowing.
func toDuration(n float64, unit time.Duration) time.Duration {
	d := n * float64(unit)
	if float64(math.MaxInt64) <= d {
		return time.Duration(math.MaxInt64)
	}
	return time.Duration(d)
}

// parseRateLimit parses the RateLimit field of draft-ietf-httpapi-ratelimit-headers,
// which lists the quota policies that apply to the request:
//
//	RateLimit: "default";r=0;t=30, "daily";r=100;t=3600
//
// It returns the longest reset time (t) among the policies with no remaining quota (r=0).
// A malformed field is ignored.
func parseRateLimit(values []string) (time.Duration, bool) {
	var (
		delay time.Duration
		found bool
	)
	for _, value := range values {
		for item := range strings.SplitSeq(value, ",") {
			remaining, reset := int64(-1), int64(-1)
			for _, param := range strings.Split(item, ";")[1:] { // the first element is the policy name
				key, val, _ := strings.Cut(strings.TrimSpace(param), "=")
				if key != "r" && key != "t" {
					continue
				}
				n, err := strconv.ParseInt(val, 10, 64)
				if err != nil || n < 0 {
					return 0, false
				}
				if key == "r" {
					remaining = n
				} else {
					reset = n
				}
			}
			if remaining != 0 || reset < 0 {
				continue
			}
			if d := toDuration(float64(reset), time.Second); !found || delay < d {
				delay, found = d, true
			}
		}
	}
	return delay, found
}

// parseRateLimitReset parses the de facto standard <prefix>-Reset header,
// which is only meaningful when the <prefix>-Remaining header doesn't report any remaining quota.
//
// Its format varies between providers, so it accepts:
//   - delay seconds (e.g. Envoy, express-rate-limit),
//   - Unix epoch seconds, which may be fractional (e.g. GitHub, GitLab, Discord),
//   - an HTTP-date or an RFC 3339 timestamp.
func parseRateLimitReset(header http.Header, prefix string, now time.Time) (time.Duration, bool) {
	value := strings.TrimSpace(header.Get(prefix + "-Reset"))
	if value == "" {
		return 0, false
	}
	if remaining := strings.TrimSpace(header.Get(prefix + "-Remaining")); remaining != "" {
		n, err := strconv.ParseInt(remaining, 10, 64)
		if err != nil || 0 < n {
			return 0, false
		}
	}
	if d, ok := parseDelay(value, time.Second); ok {
		// Nobody asks for a delay of decades, so values this large are Unix timestamps.
		// 1e9 seconds after the Unix epoch is 2001-09-09.
		const minUnixTimestamp = 1e9 * time.Second
		if minUnixTimestamp <= d {
			return time.Unix(0, 0).Add(d).Sub(now), true
		}
		return d, true
	}
	if t, err := http.ParseTime(value); err == nil {
		return t.Sub(now), true
	}
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t.Sub(now), true
	}
	return 0, false
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

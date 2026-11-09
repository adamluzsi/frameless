package resilience_test

import (
	"context"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.llib.dev/frameless/pkg/httpkit"
	"go.llib.dev/frameless/pkg/resilience"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/clock/timecop"
	"go.llib.dev/testcase/let"
	"go.llib.dev/testcase/tchttp"
)

// ExampleHTTPRoundTripper demonstrates how to use HTTPRoundTripper
// as the transport for an http.Client, enabling automatic retries
// for recoverable errors like 5xx status codes and network timeouts.
func ExampleHTTPRoundTripper() {
	// Wrap it into an http.Client to make requests with automatic retries.
	client := &http.Client{
		// Create an HTTPRoundTripper with default settings.
		// It will retry on recoverable network errors and 5xx status codes
		// using the default retry strategy.
		Transport: resilience.HTTPRoundTripper{},
	}

	req, _ := http.NewRequest(http.MethodGet, "https://example.com", nil)
	resp, err := client.Do(req)
	_ = resp
	_ = err
}

// ExampleHTTPRoundTripper_retryAfter demonstrates how to bound the delay
// that HTTPRoundTripper waits when a server asks the client to back off,
// for example with a 429 Too Many Requests response and a Retry-After header.
func ExampleHTTPRoundTripper_retryAfter() {
	client := &http.Client{
		Transport: resilience.HTTPRoundTripper{
			// Wait at most 30 seconds before retrying when the server asks for it.
			// If the server asks for a longer delay,
			// its response is returned instead of retrying too soon.
			MaxRetryAfter: 30 * time.Second,
		},
		// The client's timeout also limits how long a retry hint can be waited for.
		Timeout: time.Minute,
	}

	resp, err := client.Get("https://example.com")
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests {
		_ = resp.Header.Get("Retry-After") // the server asked for a longer delay than we were willing to wait
	}
}

func TestHTTPRoundTripper(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Before(func(t *testcase.T) {
		timecop.SetSpeed(t, 1024000)
	})

	var (
		requestBody  = let.String(s)
		responseBody = let.String(s)
		responseCode = let.OneOf(s,
			http.StatusOK,
			http.StatusCreated,
			http.StatusAccepted,
			http.StatusTeapot,
		)
		handler = testcase.Let(s, func(t *testcase.T) http.HandlerFunc {
			return func(w http.ResponseWriter, r *http.Request) {
				defer r.Body.Close()
				bs, err := io.ReadAll(r.Body)
				if err != nil {
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				assert.Should(t).Contains(string(bs), requestBody.Get(t))
				w.WriteHeader(responseCode.Get(t))
				_, _ = w.Write([]byte(responseBody.Get(t)))
			}
		})
		server = tchttp.LetServer(s, func(t *testcase.T) http.Handler {
			return handler.Get(t)
		})
	)

	var (
		transport = testcase.LetValue[http.RoundTripper](s, nil)
		retryRT   = testcase.Let(s, func(t *testcase.T) resilience.HTTPRoundTripper {
			return resilience.HTTPRoundTripper{
				Transport: transport.Get(t),
			}
		})
		client = testcase.Let(s, func(t *testcase.T) *http.Client {
			c := server.Get(t).Client()
			c.Transport = retryRT.Get(t)
			return c
		})
		method = testcase.LetValue(s, http.MethodGet)
		req    = testcase.Let(s, func(t *testcase.T) *http.Request {
			type NonResettableBody struct{ io.Reader } // To forcefully prevent resetting of the request body by the http.Client
			request, err := http.NewRequest(method.Get(t), server.Get(t).URL,
				NonResettableBody{Reader: strings.NewReader(requestBody.Get(t))})
			assert.Must(t).NoError(err)
			return request
		})
	)
	act := func(t *testcase.T) (*http.Response, error) {
		return client.Get(t).Do(req.Get(t))
	}

	getBody := func(tb testing.TB, req *http.Response) string {
		defer req.Body.Close()
		bs, err := io.ReadAll(req.Body)
		assert.NoError(tb, err)
		return string(bs)
	}

	s.Before(func(t *testcase.T) {
		timecop.SetSpeed(t, math.MaxFloat64/1000)
	})

	s.Context("it behaves as a http round tripper middleware would",
		tchttp.RoundTripperMiddleware(func(t *testcase.T, next http.RoundTripper) http.RoundTripper {
			return resilience.HTTPRoundTripper{Transport: next}
		}, tchttp.RoundTripperMiddlewareOption{
			Response: func(t *testcase.T) *http.Response {
				return &http.Response{
					StatusCode: http.StatusOK,
				}
			},
		}).Spec)

	s.Then("server response received back", func(t *testcase.T) {
		response, err := act(t)
		assert.Must(t).NoError(err)
		assert.Must(t).Equal(responseCode.Get(t), response.StatusCode)
		assert.Must(t).Equal(responseBody.Get(t), getBody(t, response))
	})

	s.When("the server responds with a recoverable error", func(s *testcase.Spec) {
		handler.Let(s, func(t *testcase.T) http.HandlerFunc {
			var once sync.Once
			return func(w http.ResponseWriter, r *http.Request) {
				bs, err := io.ReadAll(r.Body)
				assert.Should(t).NoError(err)
				assert.Should(t).Contains(string(bs), requestBody.Get(t))

				var done bool
				once.Do(func() {
					code := t.Random.Pick([]int{
						http.StatusInternalServerError,
						http.StatusBadGateway,
						http.StatusServiceUnavailable,
						http.StatusGatewayTimeout,
						http.StatusInsufficientStorage,
						http.StatusTooManyRequests,
						http.StatusRequestTimeout,
					}).(int)
					w.WriteHeader(code)
					done = true
				})
				if done {
					return
				}
				w.WriteHeader(responseCode.Get(t))
				_, _ = w.Write([]byte(responseBody.Get(t)))
			}
		})

		s.Then("eventually the request succeeds", func(t *testcase.T) {
			response, err := act(t)
			assert.Must(t).NoError(err)
			assert.Must(t).Equal(responseCode.Get(t), response.StatusCode)
			assert.Must(t).Equal(responseBody.Get(t), getBody(t, response))
		})

		s.And("if the recoverable just keep occurring too many times", func(s *testcase.Spec) {
			responseCode.Let(s, func(t *testcase.T) int {
				return t.Random.Pick([]int{
					http.StatusInternalServerError,
					http.StatusBadGateway,
					http.StatusServiceUnavailable,
					http.StatusGatewayTimeout,
					http.StatusInsufficientStorage,
				}).(int)
			})

			handler.Let(s, func(t *testcase.T) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(responseCode.Get(t))
				}
			})

			s.Then("the retry eventually fails and the bad code is returned", func(t *testcase.T) {
				response, err := act(t)
				assert.Must(t).NoError(err)
				assert.Must(t).Equal(responseCode.Get(t), response.StatusCode)
			})
		})
	})

	s.When("network encounters a timeout error", func(s *testcase.Spec) {
		transport.Let(s, func(t *testcase.T) http.RoundTripper {
			var once sync.Once
			return httpkit.RoundTripperFunc(func(r *http.Request) (*http.Response, error) {
				var err error
				once.Do(func() { err = NetTimeoutError{} })
				if err != nil {
					return nil, err
				}
				return http.DefaultTransport.RoundTrip(r)
			})
		})

		s.Then("eventually the request succeeds", func(t *testcase.T) {
			response, err := act(t)
			assert.Must(t).NoError(err)
			assert.Must(t).Equal(responseCode.Get(t), response.StatusCode)
			assert.Must(t).Equal(responseBody.Get(t), getBody(t, response))
		})
	})

	s.When("network closed error occurs", func(s *testcase.Spec) {
		transport.Let(s, func(t *testcase.T) http.RoundTripper {
			var once sync.Once
			return httpkit.RoundTripperFunc(func(r *http.Request) (*http.Response, error) {
				var err error
				once.Do(func() { err = net.ErrClosed })
				if err != nil {
					return nil, err
				}
				return http.DefaultTransport.RoundTrip(r)
			})
		})

		s.Then("eventually the request succeeds", func(t *testcase.T) {
			response, err := act(t)
			assert.Must(t).NoError(err)
			assert.Must(t).Equal(responseCode.Get(t), response.StatusCode)
			assert.Must(t).Equal(responseBody.Get(t), getBody(t, response))
		})
	})

	s.When("HTTP handler timeout error occurs", func(s *testcase.Spec) {
		transport.Let(s, func(t *testcase.T) http.RoundTripper {
			var once sync.Once
			return httpkit.RoundTripperFunc(func(r *http.Request) (*http.Response, error) {
				var err error
				once.Do(func() { err = http.ErrHandlerTimeout })
				if err != nil {
					return nil, err
				}
				return http.DefaultTransport.RoundTrip(r)
			})
		})

		s.Then("eventually the request succeeds", func(t *testcase.T) {
			response, err := act(t)
			assert.Must(t).NoError(err)
			assert.Must(t).Equal(responseCode.Get(t), response.StatusCode)
			assert.Must(t).Equal(responseBody.Get(t), getBody(t, response))
		})
	})

	s.When("the response body fails while being read", func(s *testcase.Spec) {
		// Use a sufficiently long response body so the injected read failure
		// can occur in the middle of the stream, exercising the resume-from-offset path.
		responseBody.Let(s, func(t *testcase.T) string {
			return t.Random.StringN(t.Random.IntBetween(256, 1024))
		})

		// bodyReadFailures is the number of responses whose body will fail
		// during Read before a healthy response body is finally served.
		bodyReadFailures := testcase.Let(s, func(t *testcase.T) int {
			return t.Random.IntBetween(1, 3)
		})

		// failAt is the byte offset at which a faulty body stops and reports a read error.
		// It may land anywhere within the stream, including at the very beginning (0).
		failAt := testcase.Let(s, func(t *testcase.T) int {
			return t.Random.IntBetween(0, len(responseBody.Get(t))-1)
		})

		// transportCalls counts how many times the underlying transport was invoked,
		// allowing the assertions to verify that the request was actually re-issued.
		transportCalls := testcase.Let(s, func(t *testcase.T) *int {
			return new(int)
		})

		transport.Let(s, func(t *testcase.T) http.RoundTripper {
			var (
				mutex     sync.Mutex
				failsLeft = bodyReadFailures.Get(t)
				failAfter = failAt.Get(t)
				callCount = transportCalls.Get(t)
			)
			return httpkit.RoundTripperFunc(func(r *http.Request) (*http.Response, error) {
				resp, err := http.DefaultTransport.RoundTrip(r)
				if err != nil {
					return resp, err
				}
				mutex.Lock()
				defer mutex.Unlock()
				*callCount++
				if 0 < failsLeft {
					failsLeft--
					resp.Body = &faultyResponseBody{
						ReadCloser: resp.Body,
						failAfter:  failAfter,
						failErr:    io.ErrUnexpectedEOF,
					}
				}
				return resp, nil
			})
		})

		s.Then("the broken stream is repaired and the full body is returned", func(t *testcase.T) {
			response, err := act(t)
			assert.Must(t).NoError(err)
			assert.Must(t).Equal(responseCode.Get(t), response.StatusCode)
			assert.Must(t).Equal(responseBody.Get(t), getBody(t, response))
		})

		s.Then("the request is re-issued through the transport once per failed body read", func(t *testcase.T) {
			response, err := act(t)
			assert.Must(t).NoError(err)
			assert.Must(t).Equal(responseBody.Get(t), getBody(t, response))

			// one initial request, plus one re-issue for each faulty body read.
			assert.Must(t).Equal(bodyReadFailures.Get(t)+1, *transportCalls.Get(t))
		})

		s.And("the failure happens before any byte was delivered", func(s *testcase.Spec) {
			failAt.Let(s, func(t *testcase.T) int {
				return 0
			})

			s.Then("the body is still recovered and fully returned", func(t *testcase.T) {
				response, err := act(t)
				assert.Must(t).NoError(err)
				assert.Must(t).Equal(responseBody.Get(t), getBody(t, response))
			})
		})

		s.And("the body keeps failing several times before it can be read", func(s *testcase.Spec) {
			bodyReadFailures.Let(s, func(t *testcase.T) int {
				return t.Random.IntBetween(3, 5)
			})

			s.Then("the round tripper keeps repairing the stream until the full body is returned", func(t *testcase.T) {
				response, err := act(t)
				assert.Must(t).NoError(err)
				assert.Must(t).Equal(responseBody.Get(t), getBody(t, response))
			})
		})

		s.And("the HTTP method is not safe to repeat", func(s *testcase.Spec) {
			method.Let(s, func(t *testcase.T) string {
				return t.Random.Pick([]string{
					http.MethodPost,
					http.MethodPut,
					http.MethodPatch,
					http.MethodDelete,
				}).(string)
			})

			s.Then("the body read failure is surfaced to the caller instead of being retried", func(t *testcase.T) {
				response, err := act(t)
				assert.Must(t).NoError(err) // the round trip itself succeeds

				defer response.Body.Close()
				_, err = io.ReadAll(response.Body)
				assert.Must(t).Error(err)

				// the body must not have been wrapped for resilient reading,
				// so the transport is invoked exactly once (no re-issue).
				assert.Must(t).Equal(1, *transportCalls.Get(t))
			})
		})
	})

	s.When("the server responds with an unrecoverable error", func(s *testcase.Spec) {
		responseCode.Let(s, func(t *testcase.T) int {
			return t.Random.Pick([]int{
				// just a few example, not all of them
				http.StatusBadRequest,
				http.StatusNotImplemented,
				http.StatusHTTPVersionNotSupported,
			}).(int)
		})

		count := testcase.LetValue(s, 0)
		handler.Let(s, func(t *testcase.T) http.HandlerFunc {
			return func(w http.ResponseWriter, r *http.Request) {
				count.Set(t, count.Get(t)+1)
				w.WriteHeader(responseCode.Get(t))
			}
		})

		s.Then("eventually the request succeeds", func(t *testcase.T) {
			response, err := act(t)
			assert.Must(t).NoError(err)
			assert.Must(t).Equal(responseCode.Get(t), response.StatusCode)
			assert.Must(t).Equal(1, count.Get(t))
		})

	})

	s.When("status code is configured to behave differently than how it is defined in the defaults", func(s *testcase.Spec) {
		responseCode.Let(s, func(t *testcase.T) int {
			return t.Random.Pick([]int{
				http.StatusUnauthorized,
				http.StatusTeapot,
			}).(int)
		})

		retryRT.Let(s, func(t *testcase.T) resilience.HTTPRoundTripper {
			rrt := retryRT.Super(t)
			rrt.OnStatus = map[int]bool{responseCode.Get(t): true}
			return rrt
		})

		s.And("the server is replying back with that status code", func(s *testcase.Spec) {
			count := testcase.LetValue(s, 0)
			handler.Let(s, func(t *testcase.T) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) {
					count.Set(t, count.Get(t)+1)
					if 2 < count.Get(t) {
						w.WriteHeader(http.StatusOK)
						return
					}
					w.WriteHeader(responseCode.Get(t))
				}
			})

			s.Then("eventually the request succeeds", func(t *testcase.T) {
				response, err := act(t)
				assert.Must(t).NoError(err)
				assert.Must(t).Equal(http.StatusOK, response.StatusCode)
				assert.Must(t).NotEqual(1, count.Get(t))
			})
		})

	})

	s.When("a retryable response asks the client to back off with a retry hint", func(s *testcase.Spec) {
		now := testcase.Let(s, func(t *testcase.T) time.Time {
			return time.Now().UTC().Truncate(time.Second)
		})
		s.Before(func(t *testcase.T) {
			// During a deep freeze, time only moves when the test travels,
			// which makes any waiting observable down to the nanosecond.
			timecop.Travel(t, now.Get(t), timecop.DeepFreeze)
		})

		var (
			throttledCode   = let.OneOf(s, http.StatusTooManyRequests, http.StatusServiceUnavailable)
			throttledHeader = testcase.Let(s, func(t *testcase.T) http.Header { return http.Header{} })
			transportCalls  = testcase.Let(s, func(t *testcase.T) *atomic.Int32 { return &atomic.Int32{} })
			// seconds is the delay that the server asks for.
			seconds = let.IntB(s, 1, 30)
			delay   = testcase.Let(s, func(t *testcase.T) time.Duration {
				return time.Duration(seconds.Get(t)) * time.Second
			})
			// serverNow is the time on the server's clock.
			serverNow = testcase.Let(s, func(t *testcase.T) time.Time { return now.Get(t) })
		)

		withHeader := func(s *testcase.Spec, key string, value func(t *testcase.T) string) {
			throttledHeader.Let(s, func(t *testcase.T) http.Header {
				h := throttledHeader.Super(t).Clone()
				h.Set(key, value(t))
				return h
			})
		}

		withServerClockSkew := func(s *testcase.Spec) {
			serverNow.Let(s, func(t *testcase.T) time.Time {
				skew := time.Duration(t.Random.IntBetween(-3600, 3600)) * time.Second
				return now.Get(t).Add(skew)
			})
			withHeader(s, "Date", func(t *testcase.T) string {
				return serverNow.Get(t).Format(http.TimeFormat)
			})
		}

		transport.Let(s, func(t *testcase.T) http.RoundTripper {
			var (
				calls  = transportCalls.Get(t)
				code   = throttledCode.Get(t)
				header = throttledHeader.Get(t)
				okCode = responseCode.Get(t)
				okBody = responseBody.Get(t)
			)
			return httpkit.RoundTripperFunc(func(r *http.Request) (*http.Response, error) {
				if calls.Add(1) == 1 {
					return &http.Response{
						StatusCode: code,
						Header:     header.Clone(),
						Body:       http.NoBody,
						Request:    r,
					}, nil
				}
				return &http.Response{
					StatusCode: okCode,
					Header:     http.Header{},
					Body:       io.NopCloser(strings.NewReader(okBody)),
					Request:    r,
				}, nil
			})
		})

		retryRT.Let(s, func(t *testcase.T) resilience.HTTPRoundTripper {
			rrt := retryRT.Super(t)
			// the retry strategy itself doesn't wait, so any waiting comes from the retry hint.
			rrt.RetryStrategy = attemptBudget(3)
			return rrt
		})

		thenItIsRetriedAfter := func(s *testcase.Spec, expectedDelay func(t *testcase.T) time.Duration) {
			s.Then("the request is retried only once the hinted delay has passed", func(t *testcase.T) {
				var (
					c, r     = client.Get(t), req.Get(t)
					calls    = transportCalls.Get(t)
					response *http.Response
					err      error
				)
				w := assert.NotWithin(t, waitProbe, func(context.Context) {
					response, err = c.Do(r)
				}, "the retry was expected to wait for the delay hinted by the server")
				assert.Equal(t, 1, int(calls.Load()), "no retry should be made while waiting")

				timecop.Travel(t, expectedDelay(t)-time.Nanosecond)
				assert.NotWithin(t, waitProbe, func(context.Context) { w.Wait() },
					"the retry was not expected before the hinted delay passed")
				assert.Equal(t, 1, int(calls.Load()))

				timecop.Travel(t, time.Nanosecond)
				assert.Within(t, timeout, func(context.Context) { w.Wait() },
					"the retry was expected right after the hinted delay passed")
				assert.NoError(t, err)
				assert.Equal(t, responseCode.Get(t), response.StatusCode)
				assert.Equal(t, responseBody.Get(t), getBody(t, response))
				assert.Equal(t, 2, int(calls.Load()))
			})
		}

		thenItIsRetriedRightAway := func(s *testcase.Spec) {
			s.Then("the request is retried without waiting for a hinted delay", func(t *testcase.T) {
				var (
					c, r     = client.Get(t), req.Get(t)
					response *http.Response
					err      error
				)
				assert.Within(t, timeout, func(context.Context) {
					response, err = c.Do(r)
				})
				assert.NoError(t, err)
				assert.Equal(t, responseCode.Get(t), response.StatusCode)
				assert.Equal(t, responseBody.Get(t), getBody(t, response))
				assert.Equal(t, 2, int(transportCalls.Get(t).Load()))
			})
		}

		thenTheResponseIsReturnedWithoutRetry := func(s *testcase.Spec) {
			s.Then("the response is returned right away without a retry", func(t *testcase.T) {
				var (
					c, r     = client.Get(t), req.Get(t)
					response *http.Response
					err      error
				)
				assert.Within(t, timeout, func(context.Context) {
					response, err = c.Do(r)
				})
				assert.NoError(t, err)
				defer response.Body.Close()
				assert.Equal(t, throttledCode.Get(t), response.StatusCode)
				assert.Equal(t, throttledHeader.Get(t), response.Header,
					"the retry hint should be available to the caller")
				assert.Equal(t, 1, int(transportCalls.Get(t).Load()))
			})
		}

		s.And("the hint is a Retry-After header with delay seconds", func(s *testcase.Spec) {
			withHeader(s, "Retry-After", func(t *testcase.T) string {
				return strconv.Itoa(seconds.Get(t))
			})

			thenItIsRetriedAfter(s, delay.Get)

			s.And("the delay is fractional", func(s *testcase.Spec) {
				withHeader(s, "Retry-After", func(t *testcase.T) string {
					return strconv.Itoa(seconds.Get(t)) + ".5"
				})

				thenItIsRetriedAfter(s, func(t *testcase.T) time.Duration {
					return delay.Get(t) + time.Second/2
				})
			})

			s.And("the delay is zero", func(s *testcase.Spec) {
				withHeader(s, "Retry-After", func(t *testcase.T) string { return "0" })

				thenItIsRetriedRightAway(s)
			})

			s.And("the response status is not retryable", func(s *testcase.Spec) {
				throttledCode.LetValue(s, http.StatusBadRequest)

				thenTheResponseIsReturnedWithoutRetry(s)
			})
		})

		s.And("the hint is a Retry-After header with an HTTP-date", func(s *testcase.Spec) {
			withHeader(s, "Retry-After", func(t *testcase.T) string {
				return serverNow.Get(t).Add(delay.Get(t)).Format(http.TimeFormat)
			})

			thenItIsRetriedAfter(s, delay.Get)

			s.And("the server's clock is out of sync with the client's", func(s *testcase.Spec) {
				withServerClockSkew(s)

				thenItIsRetriedAfter(s, delay.Get)
			})

			s.And("the date is already in the past", func(s *testcase.Spec) {
				withHeader(s, "Retry-After", func(t *testcase.T) string {
					return serverNow.Get(t).Add(-delay.Get(t)).Format(http.TimeFormat)
				})

				thenItIsRetriedRightAway(s)
			})
		})

		s.And("the hint is a Retry-After header in milliseconds", func(s *testcase.Spec) {
			var (
				headerKey = let.OneOf(s, "Retry-After-Ms", "X-Ms-Retry-After-Ms")
				millis    = let.IntB(s, 1, 30_000)
			)
			throttledHeader.Let(s, func(t *testcase.T) http.Header {
				h := throttledHeader.Super(t).Clone()
				h.Set(headerKey.Get(t), strconv.Itoa(millis.Get(t)))
				return h
			})
			millisDelay := func(t *testcase.T) time.Duration {
				return time.Duration(millis.Get(t)) * time.Millisecond
			}

			thenItIsRetriedAfter(s, millisDelay)

			s.And("a Retry-After header with delay seconds is also present", func(s *testcase.Spec) {
				withHeader(s, "Retry-After", func(t *testcase.T) string {
					return strconv.Itoa(t.Random.IntBetween(31, 60))
				})

				thenItIsRetriedAfter(s, millisDelay) // the more precise millisecond hint takes precedence
			})
		})

		s.And("the hint is an IETF RateLimit header with an exhausted quota policy", func(s *testcase.Spec) {
			withHeader(s, "RateLimit", func(t *testcase.T) string {
				return fmt.Sprintf(`"default";r=0;t=%d`, seconds.Get(t))
			})

			thenItIsRetriedAfter(s, delay.Get)

			s.And("multiple policies are listed", func(s *testcase.Spec) {
				withHeader(s, "RateLimit", func(t *testcase.T) string {
					return fmt.Sprintf(`"burst";r=0;t=1, "daily";r=0;t=%d, "hourly";r=42;t=3600`, seconds.Get(t))
				})

				// the quota is only available again once every exhausted policy is reset.
				thenItIsRetriedAfter(s, delay.Get)
			})

			s.And("the policy still has quota remaining", func(s *testcase.Spec) {
				withHeader(s, "RateLimit", func(t *testcase.T) string {
					return fmt.Sprintf(`"default";r=%d;t=%d`, t.Random.IntBetween(1, 100), seconds.Get(t))
				})

				thenItIsRetriedRightAway(s)
			})

			s.And("the field is malformed", func(s *testcase.Spec) {
				withHeader(s, "RateLimit", func(t *testcase.T) string {
					return fmt.Sprintf(`"default";r=none;t=%d`, seconds.Get(t))
				})

				thenItIsRetriedRightAway(s)
			})

			s.And("a Retry-After header is also present", func(s *testcase.Spec) {
				withHeader(s, "Retry-After", func(t *testcase.T) string {
					return strconv.Itoa(seconds.Get(t) + 30)
				})

				thenItIsRetriedAfter(s, func(t *testcase.T) time.Duration {
					return delay.Get(t) + 30*time.Second // Retry-After takes precedence
				})
			})
		})

		s.And("the hint is a rate limit reset header", func(s *testcase.Spec) {
			var (
				prefix    = let.OneOf(s, "RateLimit", "X-RateLimit")
				reset     = testcase.Let(s, func(t *testcase.T) string { return strconv.Itoa(seconds.Get(t)) })
				remaining = testcase.LetValue(s, "0") // empty means that the header is absent
			)
			throttledHeader.Let(s, func(t *testcase.T) http.Header {
				h := throttledHeader.Super(t).Clone()
				h.Set(prefix.Get(t)+"-Reset", reset.Get(t))
				if v := remaining.Get(t); v != "" {
					h.Set(prefix.Get(t)+"-Remaining", v)
				}
				return h
			})

			thenItIsRetriedAfter(s, delay.Get)

			s.And("the remaining quota is not reported", func(s *testcase.Spec) {
				remaining.LetValue(s, "")

				thenItIsRetriedAfter(s, delay.Get)
			})

			s.And("there is still quota remaining", func(s *testcase.Spec) {
				remaining.Let(s, func(t *testcase.T) string {
					return strconv.Itoa(t.Random.IntBetween(1, 100))
				})

				thenItIsRetriedRightAway(s)
			})

			s.And("the reset time is a Unix timestamp", func(s *testcase.Spec) {
				withServerClockSkew(s)
				reset.Let(s, func(t *testcase.T) string {
					return strconv.FormatInt(serverNow.Get(t).Add(delay.Get(t)).Unix(), 10)
				})

				thenItIsRetriedAfter(s, delay.Get)

				s.And("it has fractional seconds", func(s *testcase.Spec) {
					reset.Let(s, func(t *testcase.T) string {
						return reset.Super(t) + ".5"
					})

					thenItIsRetriedAfter(s, func(t *testcase.T) time.Duration {
						return delay.Get(t) + time.Second/2
					})
				})
			})

			s.And("the reset time is a timestamp", func(s *testcase.Spec) {
				withServerClockSkew(s)
				reset.Let(s, func(t *testcase.T) string {
					resetAt := serverNow.Get(t).Add(delay.Get(t))
					return t.Random.Pick([]string{
						resetAt.Format(http.TimeFormat),
						resetAt.Format(time.RFC3339),
					}).(string)
				})

				thenItIsRetriedAfter(s, delay.Get)
			})
		})

		s.And("the hint is malformed", func(s *testcase.Spec) {
			withHeader(s, "Retry-After", func(t *testcase.T) string {
				return t.Random.Pick([]string{"soon", "-5", "NaN", "Inf", "tomorrow 10:00"}).(string)
			})

			thenItIsRetriedRightAway(s)
		})

		s.And("the hinted delay is longer than what HTTPRoundTripper waits by default", func(s *testcase.Spec) {
			withHeader(s, "Retry-After", func(t *testcase.T) string {
				return strconv.Itoa(t.Random.IntBetween(61, 3600))
			})

			thenTheResponseIsReturnedWithoutRetry(s)
		})

		s.And("the hinted delay is exactly what HTTPRoundTripper waits at most by default", func(s *testcase.Spec) {
			seconds.LetValue(s, 60)
			withHeader(s, "Retry-After", func(t *testcase.T) string {
				return strconv.Itoa(seconds.Get(t))
			})

			thenItIsRetriedAfter(s, delay.Get)
		})

		s.And("MaxRetryAfter is configured", func(s *testcase.Spec) {
			maxRetryAfter := testcase.Let(s, func(t *testcase.T) time.Duration {
				return time.Duration(t.Random.IntBetween(5, 120)) * time.Second
			})
			retryRT.Let(s, func(t *testcase.T) resilience.HTTPRoundTripper {
				rrt := retryRT.Super(t)
				rrt.MaxRetryAfter = maxRetryAfter.Get(t)
				return rrt
			})

			s.And("the hinted delay is within it", func(s *testcase.Spec) {
				seconds.Let(s, func(t *testcase.T) int {
					return t.Random.IntBetween(1, int(maxRetryAfter.Get(t)/time.Second))
				})
				withHeader(s, "Retry-After", func(t *testcase.T) string {
					return strconv.Itoa(seconds.Get(t))
				})

				thenItIsRetriedAfter(s, delay.Get)
			})

			s.And("the hinted delay is longer than it", func(s *testcase.Spec) {
				withHeader(s, "Retry-After", func(t *testcase.T) string {
					return strconv.Itoa(int(maxRetryAfter.Get(t)/time.Second) + t.Random.IntBetween(1, 60))
				})

				thenTheResponseIsReturnedWithoutRetry(s)
			})
		})

		s.And("retry hints are disabled with a negative MaxRetryAfter", func(s *testcase.Spec) {
			retryRT.Let(s, func(t *testcase.T) resilience.HTTPRoundTripper {
				rrt := retryRT.Super(t)
				rrt.MaxRetryAfter = -1
				return rrt
			})
			withHeader(s, "Retry-After", func(t *testcase.T) string {
				return strconv.Itoa(seconds.Get(t))
			})

			thenItIsRetriedRightAway(s)
		})

		s.And("the hinted delay outlasts the request's deadline", func(s *testcase.Spec) {
			client.Let(s, func(t *testcase.T) *http.Client {
				c := client.Super(t)
				c.Timeout = 10 * time.Second // http.Client#Timeout sets the deadline of the request's context
				return c
			})
			withHeader(s, "Retry-After", func(t *testcase.T) string {
				return strconv.Itoa(t.Random.IntBetween(11, 60))
			})

			thenTheResponseIsReturnedWithoutRetry(s)
		})

		s.And("the request is cancelled while waiting for the hinted delay", func(s *testcase.Spec) {
			ctx, cancel := let.ContextWithCancel(s)
			req.Let(s, func(t *testcase.T) *http.Request {
				return req.Super(t).WithContext(ctx.Get(t))
			})
			withHeader(s, "Retry-After", func(t *testcase.T) string {
				return strconv.Itoa(seconds.Get(t))
			})

			s.Then("the waiting is interrupted, and the cancellation is reported", func(t *testcase.T) {
				var (
					c, r = client.Get(t), req.Get(t)
					err  error
				)
				w := assert.NotWithin(t, waitProbe, func(context.Context) {
					_, err = c.Do(r)
				})

				cancel.Get(t)()
				assert.Within(t, timeout, func(context.Context) { w.Wait() })
				assert.ErrorIs(t, err, context.Canceled)
				assert.Equal(t, 1, int(transportCalls.Get(t).Load()))
			})
		})
	})
}

type NetTimeoutError struct{}

func (NetTimeoutError) Error() string { return "net: timeout error" }
func (NetTimeoutError) Timeout() bool { return true }

// attemptBudget is a RetryStrategy that allows a fixed number of attempts without waiting between them.
type attemptBudget int

func (n attemptBudget) ShouldTry(ctx context.Context, attempt resilience.RetryAttempt) bool {
	return ctx.Err() == nil && attempt.FailureCount < int(n)
}

// waitProbe is how long a test watches a request to confirm that it is still waiting.
// Time is deep frozen while it watches, so a request that doesn't wait finishes almost instantly.
const waitProbe = timeout / 5

// faultyResponseBody is an http.Response body that simulates a connection
// dropping mid-stream: after delivering failAfter bytes, the next Read returns
// failErr once instead of more data. It is used to exercise HTTPRoundTripper's
// resilient response-body reading, which re-issues the request and resumes the
// stream from the current offset for repeatable HTTP methods.
type faultyResponseBody struct {
	io.ReadCloser
	failAfter int
	failErr   error
	delivered int
	failed    bool
}

func (b *faultyResponseBody) Read(p []byte) (int, error) {
	if !b.failed && b.failAfter <= b.delivered {
		b.failed = true
		return 0, b.failErr
	}
	if !b.failed {
		// never deliver past failAfter in a single read,
		// so the failure reliably lands at the intended offset.
		if remaining := b.failAfter - b.delivered; remaining < len(p) {
			p = p[:remaining]
		}
	}
	n, err := b.ReadCloser.Read(p)
	b.delivered += n
	return n, err
}

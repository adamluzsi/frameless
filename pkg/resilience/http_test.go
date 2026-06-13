package resilience_test

import (
	"io"
	"math"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"

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
}

type NetTimeoutError struct{}

func (NetTimeoutError) Error() string { return "net: timeout error" }
func (NetTimeoutError) Timeout() bool { return true }

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

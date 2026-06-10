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

func TestRoundTripper(t *testing.T) {
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
		req = testcase.Let(s, func(t *testcase.T) *http.Request {
			type NonResettableBody struct{ io.Reader } // To forcefully prevent resetting of the request body by the http.Client
			request, err := http.NewRequest(http.MethodGet, server.Get(t).URL,
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

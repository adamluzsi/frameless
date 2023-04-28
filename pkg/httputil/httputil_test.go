package httputil_test

import (
	"github.com/adamluzsi/frameless/pkg/httputil"
	"github.com/adamluzsi/frameless/pkg/logger"
	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/assert"
	"github.com/adamluzsi/testcase/clock/timecop"
	"github.com/adamluzsi/testcase/httpspec"
	"github.com/adamluzsi/testcase/let"
	"github.com/adamluzsi/testcase/random"
	"io"
	"math"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRoundTripperFunc(t *testing.T) {
	s := testcase.NewSpec(t)
	httpspec.ItBehavesLikeRoundTripperMiddleware(s, func(t *testcase.T, next http.RoundTripper) http.RoundTripper {
		return httputil.RoundTripperFunc(func(r *http.Request) (*http.Response, error) {
			return next.RoundTrip(r)
		})
	})
}

func TestRetryRoundTripper(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Before(func(t *testcase.T) {
		timecop.SetSpeed(t, 1024000)
	})

	var (
		requestBody  = let.String(s)
		responseBody = let.String(s)
		responseCode = let.ElementFrom(s,
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
				assert.Should(t).Contain(string(bs), requestBody.Get(t))
				w.WriteHeader(responseCode.Get(t))
				_, _ = w.Write([]byte(responseBody.Get(t)))
			}
		})
		server = httpspec.LetServer(s, func(t *testcase.T) http.Handler {
			return handler.Get(t)
		})
	)

	var (
		transport = testcase.LetValue[http.RoundTripper](s, nil)
		client    = testcase.Let(s, func(t *testcase.T) *http.Client {
			c := server.Get(t).Client()
			c.Transport = httputil.RetryRoundTripper{
				Transport: transport.Get(t),
			}
			return c
		})
		req = testcase.Let(s, func(t *testcase.T) *http.Request {
			type NonResettableBody struct{ io.Reader } // To forcefully prevent resetting of the request body by the http.Client
			request, err := http.NewRequest(http.MethodGet, server.Get(t).URL,
				NonResettableBody{Reader: strings.NewReader(requestBody.Get(t))})
			t.Must.NoError(err)
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

	s.Context("", func(s *testcase.Spec) {
		httpspec.Response.Let(s, func(t *testcase.T) *http.Response {
			r := httpspec.Response.Super(t)
			r.StatusCode = http.StatusOK
			return r
		})

		httpspec.ItBehavesLikeRoundTripperMiddleware(s, func(t *testcase.T, next http.RoundTripper) http.RoundTripper {
			return httputil.RetryRoundTripper{Transport: next}
		})
	})

	s.Then("server response received back", func(t *testcase.T) {
		response, err := act(t)
		t.Must.NoError(err)
		t.Must.Equal(responseCode.Get(t), response.StatusCode)
		t.Must.Equal(responseBody.Get(t), getBody(t, response))
	})

	s.When("the server responds with a recoverable error", func(s *testcase.Spec) {
		handler.Let(s, func(t *testcase.T) http.HandlerFunc {
			var once sync.Once
			return func(w http.ResponseWriter, r *http.Request) {
				bs, err := io.ReadAll(r.Body)
				assert.Should(t).NoError(err)
				assert.Should(t).Contain(string(bs), requestBody.Get(t))

				var done bool
				once.Do(func() {
					code := t.Random.SliceElement([]int{
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
			t.Must.NoError(err)
			t.Must.Equal(responseCode.Get(t), response.StatusCode)
			t.Must.Equal(responseBody.Get(t), getBody(t, response))
		})

		s.And("if the recoverable just keep occuring too many times", func(s *testcase.Spec) {
			responseCode.Let(s, func(t *testcase.T) int {
				return t.Random.SliceElement([]int{
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
				t.Must.NoError(err)
				t.Must.Equal(responseCode.Get(t), response.StatusCode)
			})
		})
	})

	s.When("network encounters a timeout error", func(s *testcase.Spec) {
		transport.Let(s, func(t *testcase.T) http.RoundTripper {
			var once sync.Once
			return httputil.RoundTripperFunc(func(r *http.Request) (*http.Response, error) {
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
			t.Must.NoError(err)
			t.Must.Equal(responseCode.Get(t), response.StatusCode)
			t.Must.Equal(responseBody.Get(t), getBody(t, response))
		})
	})

	s.When("network closed error occurs", func(s *testcase.Spec) {
		transport.Let(s, func(t *testcase.T) http.RoundTripper {
			var once sync.Once
			return httputil.RoundTripperFunc(func(r *http.Request) (*http.Response, error) {
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
			t.Must.NoError(err)
			t.Must.Equal(responseCode.Get(t), response.StatusCode)
			t.Must.Equal(responseBody.Get(t), getBody(t, response))
		})
	})

	s.When("HTTP handler timeout error occurs", func(s *testcase.Spec) {
		transport.Let(s, func(t *testcase.T) http.RoundTripper {
			var once sync.Once
			return httputil.RoundTripperFunc(func(r *http.Request) (*http.Response, error) {
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
			t.Must.NoError(err)
			t.Must.Equal(responseCode.Get(t), response.StatusCode)
			t.Must.Equal(responseBody.Get(t), getBody(t, response))
		})
	})

	s.When("the server responds with an unrecoverable error", func(s *testcase.Spec) {
		responseCode.Let(s, func(t *testcase.T) int {
			return t.Random.SliceElement([]int{
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
			t.Must.NoError(err)
			t.Must.Equal(responseCode.Get(t), response.StatusCode)
			t.Must.Equal(1, count.Get(t))
		})
	})
}

type NetTimeoutError struct{}

func (NetTimeoutError) Error() string { return "net: timeout error" }
func (NetTimeoutError) Timeout() bool { return true }

func TestAccessLog_smoke(t *testing.T) {
	rnd := random.New(random.CryptoSeed{})

	var responseCode = rnd.SliceElement([]int{
		http.StatusTeapot,
		http.StatusOK,
		http.StatusCreated,
		http.StatusAccepted,
		http.StatusInternalServerError,
	}).(int)

	var (
		requestMethod = rnd.SliceElement([]string{http.MethodGet, http.MethodDelete, http.MethodPost}).(string)
		requestBody   = rnd.String()
		responseBody  = rnd.String()
		gotRemoteAddress string
		logs []logger.Fields
	)
	
	logger.Stub(t)

	logger.Default.Hijack = func(level logger.Level, msg string, fields logger.Fields) {
		logs = append(logs, fields)
	}
	
	now := time.Now()
	
	timecop.Travel(t, now, timecop.Freeze())

	handler := &httputil.AccessLog{
		Next: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// this operation takes 1.542s 
			timecop.Travel(t, time.Second + 542*time.Millisecond, timecop.Freeze())
			
			gotRemoteAddress = r.RemoteAddr

			should := assert.Should(t)
			defer r.Body.Close()
			bs, err := io.ReadAll(r.Body)
			should.NoError(err)
			if err == nil {
				should.Contain(string(bs), requestBody)
			}
			should.Equal(requestMethod, r.Method)
			w.WriteHeader(responseCode)
			w.Write([]byte(responseBody))
		}),
		AdditionalLoggingDetail: func(w http.ResponseWriter, r *http.Request) logger.LoggingDetail {
			return logger.Field("foo", "baz")
		},
	}

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	request, err := http.NewRequest(requestMethod, server.URL, strings.NewReader(requestBody))
	assert.NoError(t, err)

	response, err := server.Client().Do(request)
	assert.NoError(t, err)

	defer response.Body.Close()
	gotResponseBody, err := io.ReadAll(response.Body)
	assert.NoError(t, err)
	assert.Contain(t, string(gotResponseBody), responseBody)
	assert.Equal(t, responseCode, response.StatusCode)
	assert.True(t, len(logs) == 1)
	u, _ := url.Parse(server.URL)
	assert.Equal(t, logs[0], logger.Fields{
		"duration":             "1.542s",
		"host":                 u.Host,
		"method":               requestMethod,
		"path":                 "/",
		"query":                "",
		"remote_address":       gotRemoteAddress,
		"status":               responseCode,
		"request_body_length":  len(requestBody),
		"response_body_length": len(responseBody),
		"foo":                  "baz",
	})

	handler.AdditionalLoggingDetail = nil
	logs = nil

	request, err = http.NewRequest(requestMethod, server.URL, strings.NewReader(requestBody))
	assert.NoError(t, err)

	response, err = server.Client().Do(request)
	assert.NoError(t, err)

	defer response.Body.Close()
	gotResponseBody, err = io.ReadAll(response.Body)
	assert.NoError(t, err)
	assert.Contain(t, string(gotResponseBody), responseBody)
	assert.Equal(t, responseCode, response.StatusCode)
	assert.True(t, len(logs) == 1)
	assert.Equal(t, logs[0], logger.Fields{
		"duration":             "1.542s",
		"host":                 u.Host,
		"method":               requestMethod,
		"path":                 "/",
		"query":                "",
		"request_body_length":  len(requestBody),
		"response_body_length": len(responseBody),
		"remote_address":       gotRemoteAddress,
		"status":               responseCode,
	})
}

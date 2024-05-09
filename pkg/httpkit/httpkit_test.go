package httpkit_test

import (
	"fmt"
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

	"go.llib.dev/frameless/pkg/httpkit"
	"go.llib.dev/frameless/pkg/logger"
	"go.llib.dev/frameless/pkg/retry"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/clock/timecop"
	"go.llib.dev/testcase/httpspec"
	"go.llib.dev/testcase/let"
	"go.llib.dev/testcase/random"
)

func Test_interfaceMultiplexer(t *testing.T) {
	var _ httpkit.Multiplexer = http.NewServeMux()
}

func TestRoundTripperFunc(t *testing.T) {
	s := testcase.NewSpec(t)
	httpspec.ItBehavesLikeRoundTripperMiddleware(s, func(t *testcase.T, next http.RoundTripper) http.RoundTripper {
		return httpkit.RoundTripperFunc(func(r *http.Request) (*http.Response, error) {
			return next.RoundTrip(r)
		})
	})
}

func ExampleRetryRoundTripper() {
	httpClient := http.Client{
		Transport: httpkit.RetryRoundTripper{
			RetryStrategy: retry.ExponentialBackoff{ // optional
				Timeout: 5 * time.Minute,
			},

			Transport: http.DefaultTransport, // optional

			OnStatus: map[int]bool{ // optional
				http.StatusTeapot:          true,
				http.StatusTooManyRequests: false,
			},
		},
	}

	httpClient.Get("https://go.llib.dev")
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
		retryRT   = testcase.Let(s, func(t *testcase.T) httpkit.RetryRoundTripper {
			return httpkit.RetryRoundTripper{
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
			return httpkit.RetryRoundTripper{Transport: next}
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
			t.Must.NoError(err)
			t.Must.Equal(responseCode.Get(t), response.StatusCode)
			t.Must.Equal(responseBody.Get(t), getBody(t, response))
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
			t.Must.NoError(err)
			t.Must.Equal(responseCode.Get(t), response.StatusCode)
			t.Must.Equal(responseBody.Get(t), getBody(t, response))
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

	s.When("status code is configured to behave differently than how it is defined in the defaults", func(s *testcase.Spec) {
		responseCode.Let(s, func(t *testcase.T) int {
			return t.Random.SliceElement([]int{
				http.StatusUnauthorized,
				http.StatusTeapot,
			}).(int)
		})

		retryRT.Let(s, func(t *testcase.T) httpkit.RetryRoundTripper {
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
				t.Must.NoError(err)
				t.Must.Equal(http.StatusOK, response.StatusCode)
				t.Must.NotEqual(1, count.Get(t))
			})
		})

	})
}

type NetTimeoutError struct{}

func (NetTimeoutError) Error() string { return "net: timeout error" }
func (NetTimeoutError) Timeout() bool { return true }

func TestAccessLog_smoke(t *testing.T) {
	rnd := random.New(random.CryptoSeed{})

	var (
		responseCode = rnd.SliceElement([]int{
			http.StatusTeapot,
			http.StatusOK,
			http.StatusCreated,
			http.StatusAccepted,
			http.StatusInternalServerError,
		}).(int)
		requestQuery     = url.Values{"foo": {rnd.StringNC(3, random.CharsetDigit())}}
		requestMethod    = rnd.SliceElement([]string{http.MethodGet, http.MethodDelete, http.MethodPost}).(string)
		requestBody      = rnd.String()
		responseBody     = rnd.String()
		gotRemoteAddress string
		logs             []logger.Fields
	)

	logger.Stub(t)

	logger.Default.Hijack = func(level logger.Level, msg string, fields logger.Fields) {
		logs = append(logs, fields)
	}

	now := time.Now()

	timecop.Travel(t, now, timecop.Freeze())

	handler := &httpkit.AccessLog{
		Next: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// this operation takes 1.542s
			timecop.Travel(t, time.Second+542*time.Millisecond, timecop.Freeze())
			gotRemoteAddress = r.RemoteAddr
			should := assert.Should(t)
			defer r.Body.Close()
			bs, err := io.ReadAll(r.Body)
			should.NoError(err)
			if err == nil {
				should.Contain(string(bs), requestBody)
			}
			should.Equal(requestMethod, r.Method)
			should.Equal(requestQuery.Encode(), r.URL.Query().Encode())
			w.WriteHeader(responseCode)
			w.Write([]byte(responseBody))
		}),
		AdditionalLoggingDetail: func(w http.ResponseWriter, r *http.Request) logger.LoggingDetail {
			return logger.Field("foo", "baz")
		},
	}

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	request, err := http.NewRequest(requestMethod, server.URL+"?"+requestQuery.Encode(), strings.NewReader(requestBody))
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
	assert.Equal(t, logger.Fields{
		"duration":             "1.542s",
		"host":                 u.Host,
		"method":               requestMethod,
		"path":                 "/",
		"query":                requestQuery.Encode(),
		"remote_address":       gotRemoteAddress,
		"status":               responseCode,
		"request_body_length":  len(requestBody),
		"response_body_length": len(responseBody),
		"foo":                  "baz",
	}, logs[0])

	handler.AdditionalLoggingDetail = nil
	logs = nil

	request, err = http.NewRequest(requestMethod, server.URL+"?"+requestQuery.Encode(), strings.NewReader(requestBody))
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
		"query":                requestQuery.Encode(),
		"request_body_length":  len(requestBody),
		"response_body_length": len(responseBody),
		"remote_address":       gotRemoteAddress,
		"status":               responseCode,
	})
}

func ExampleMount() {
	var (
		apiV0 http.Handler
		webUI http.Handler
		mux   = http.NewServeMux()
	)
	httpkit.Mount(mux, "/api/v0", apiV0)
	httpkit.Mount(mux, "/ui", webUI)
}

func TestMount(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		serveMux = testcase.Let(s, func(t *testcase.T) *http.ServeMux { return http.NewServeMux() })
		pattern  = testcase.LetValue[string](s, "/path")
		lastReq  = testcase.LetValue[*http.Request](s, nil)
		handler  = testcase.Let[http.Handler](s, func(t *testcase.T) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				lastReq.Set(t, r)
				w.WriteHeader(http.StatusTeapot)
			})
		})
	)
	act := func(t *testcase.T) {
		httpkit.Mount(
			serveMux.Get(t),
			pattern.Get(t),
			handler.Get(t),
		)
	}

	makeRequest := func(t *testcase.T, path string) *http.Response {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, path, nil)
		serveMux.Get(t).ServeHTTP(w, r)
		return w.Result()
	}

	s.When("mount pattern is not in a clean format", func(s *testcase.Spec) {
		pattern.LetValue(s, "//a/b/c/..")

		s.Then("mounted to the correct path", func(t *testcase.T) {
			act(t)

			response := makeRequest(t, "/a/b")
			t.Must.Equal(http.StatusTeapot, response.StatusCode)
		})
	})

	s.When(`pattern lack trailing slash`, func(s *testcase.Spec) {
		pattern.LetValue(s, `/path0`)

		s.Then(`it will be still available to call even for the under paths`, func(t *testcase.T) {
			act(t)

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, `/path0/123`, nil)
			serveMux.Get(t).ServeHTTP(w, r)

			t.Must.Equal(http.StatusTeapot, w.Result().StatusCode)
		})
	})

	s.When(`pattern lack leading slash`, func(s *testcase.Spec) {
		pattern.LetValue(s, `path1/`)

		s.Then(`it will be still available to call even for the under paths`, func(t *testcase.T) {
			act(t)

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, `/path1/123`, nil)
			serveMux.Get(t).ServeHTTP(w, r)
			t.Must.Equal(http.StatusTeapot, w.Result().StatusCode)
		})
	})

	s.When(`pattern lack leading and trailing slash`, func(s *testcase.Spec) {
		pattern.LetValue(s, `path2`)

		s.Then(`it will be still available to call even for the under paths`, func(t *testcase.T) {
			act(t)

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, `/path2/123`, nil)
			serveMux.Get(t).ServeHTTP(w, r)
			t.Must.Equal(http.StatusTeapot, w.Result().StatusCode)
		})
	})

	s.When(`pattern includes nested path`, func(s *testcase.Spec) {
		pattern.LetValue(s, `/test/this/out/`)

		s.Then(`it will be still available to call even for the under paths`, func(t *testcase.T) {
			act(t)

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, `/test/this/out/123`, nil)
			serveMux.Get(t).ServeHTTP(w, r)
			t.Must.Equal(http.StatusTeapot, w.Result().StatusCode)
		})
	})
}

func TestWithMiddleware_order(t *testing.T) {
	var elements []string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte(fmt.Sprintf("%#v", elements)))
	})

	combined := httpkit.WithMiddleware(handler,
		func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				elements = append(elements, "foo")
				next.ServeHTTP(w, r)
			})
		},
		func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				elements = append(elements, "bar")
				next.ServeHTTP(w, r)
			})
		},
		func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				elements = append(elements, "baz")
				next.ServeHTTP(w, r)
			})
		},
	)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	combined.ServeHTTP(rr, req)
	assert.Equal(t,
		fmt.Sprintf("%#v", []string{"foo", "bar", "baz"}),
		rr.Body.String())
}

func TestWithMiddleware_nilIsIgnored(t *testing.T) {
	t.Log("order of the passed in middleware are representing how it will be called")

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})

	combined := httpkit.WithMiddleware(handler,
		func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				next.ServeHTTP(w, r)
			})
		},
		nil, // intentionally nil
		func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				next.ServeHTTP(w, r)
			})
		},
	)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	combined.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusTeapot, rr.Code)
}

func ExampleWithRoundTripper() {

	transport := httpkit.WithRoundTripper(nil, func(next http.RoundTripper) http.RoundTripper {
		return httpkit.RoundTripperFunc(func(request *http.Request) (*http.Response, error) {
			request.Header.Set("Authorization", "<type> <credentials>")

			return next.RoundTrip(request)
		})
	})

	_ = &http.Client{
		Transport: transport,
	}

}

func TestWithRoundTripper(t *testing.T) {
	t.Run("order", func(t *testing.T) {
		var elements []string
		transport := httpkit.RoundTripperFunc(func(request *http.Request) (*http.Response, error) {
			const code = http.StatusTeapot
			assert.NotEmpty(t, elements)
			return &http.Response{
				Status:     http.StatusText(code),
				StatusCode: code,
				Proto:      "HTTP/1.0",
				ProtoMajor: 1,
				ProtoMinor: 0,
			}, nil
		})

		combined := httpkit.WithRoundTripper(transport,
			func(next http.RoundTripper) http.RoundTripper {
				return httpkit.RoundTripperFunc(func(r *http.Request) (*http.Response, error) {
					elements = append(elements, "foo")
					return next.RoundTrip(r)
				})
			},
			func(next http.RoundTripper) http.RoundTripper {
				return httpkit.RoundTripperFunc(func(r *http.Request) (*http.Response, error) {
					elements = append(elements, "bar")
					return next.RoundTrip(r)
				})
			},
			func(next http.RoundTripper) http.RoundTripper {
				return httpkit.RoundTripperFunc(func(r *http.Request) (*http.Response, error) {
					elements = append(elements, "baz")
					return next.RoundTrip(r)
				})
			},
		)

		req := httptest.NewRequest(http.MethodGet, "https://go.llib.dev", nil)
		resp, err := combined.RoundTrip(req)
		assert.NoError(t, err)
		assert.Equal(t, resp.StatusCode, http.StatusTeapot)
		assert.Equal(t, elements, []string{"foo", "bar", "baz"})
	})

	t.Run("request mutation in the pipeline is possible", func(t *testing.T) {
		const HeaderKey = "X-TEST"
		transport := httpkit.RoundTripperFunc(func(request *http.Request) (*http.Response, error) {
			const code = http.StatusTeapot
			assert.NotEmpty(t, request.Header.Get(HeaderKey), "OK")
			return &http.Response{
				Status:     http.StatusText(code),
				StatusCode: code,
				Proto:      "HTTP/1.0",
				ProtoMajor: 1,
				ProtoMinor: 0,
			}, nil
		})

		combined := httpkit.WithRoundTripper(transport,
			func(next http.RoundTripper) http.RoundTripper {
				return httpkit.RoundTripperFunc(func(r *http.Request) (*http.Response, error) {
					r.Header.Set(HeaderKey, "OK")
					return next.RoundTrip(r)
				})
			},
		)

		req := httptest.NewRequest(http.MethodGet, "https://go.llib.dev", nil)
		resp, err := combined.RoundTrip(req)
		assert.NoError(t, err)
		assert.Equal(t, resp.StatusCode, http.StatusTeapot)
	})

	t.Run("on nil transport, default transport is used", func(t *testing.T) {
		og := http.DefaultTransport
		defer func() { http.DefaultTransport = og }()

		http.DefaultTransport = httpkit.RoundTripperFunc(func(request *http.Request) (*http.Response, error) {
			const code = http.StatusTeapot
			return &http.Response{
				Status:     http.StatusText(code),
				StatusCode: code,
				Proto:      "HTTP/1.0",
				ProtoMajor: 1,
				ProtoMinor: 0,
			}, nil
		})

		combined := httpkit.WithRoundTripper(nil,
			func(next http.RoundTripper) http.RoundTripper {
				assert.NotNil(t, next)
				return httpkit.RoundTripperFunc(func(r *http.Request) (*http.Response, error) {
					return next.RoundTrip(r)
				})
			},
		)

		req := httptest.NewRequest(http.MethodGet, "https://go.llib.dev", nil)
		resp, err := combined.RoundTrip(req)
		assert.NoError(t, err)
		assert.Equal(t, resp.StatusCode, http.StatusTeapot)
	})
}

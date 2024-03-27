package tasker_test

import (
	"context"
	"go.llib.dev/frameless/pkg/tasker"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/clock"
	"go.llib.dev/testcase/clock/timecop"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

func ExampleHTTPServerTask() {
	srv := &http.Server{
		Addr:    "localhost:58080",
		Handler: http.NewServeMux(),
	}

	tasker.HTTPServerTask(srv).
		Run(context.Background())
}

func ExampleHTTPServerTask_withMain() {
	srv := &http.Server{Handler: http.NewServeMux()}

	tasker.Main(context.Background(),
		tasker.HTTPServerTask(srv,
			tasker.HTTPServerPortFromENV("PORT", "LOYALIFY_WEB_PORT")))
}

func TestHTTPServerTask_gracefulShutdown(t *testing.T) {
	var inFlight int64
	h := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		atomic.AddInt64(&inFlight, 1)
		defer atomic.AddInt64(&inFlight, -1)

		writer.WriteHeader(http.StatusTeapot)
		if v := request.URL.Query().Get("wait"); v != "" {
			<-clock.After(time.Minute)
		}
		_, _ = writer.Write([]byte("Hello, world!"))
	})
	srv := &http.Server{
		Addr:    "localhost:58080",
		Handler: h,
	}
	const srvURL = "http://localhost:58080/"

	ctx, cancel := context.WithCancel(context.Background())

	go func() { assert.NoError(t, tasker.HTTPServerTask(srv).Run(ctx)) }()

	eventually := assert.MakeRetry(5 * time.Second)

	eventually.Assert(t, func(it assert.It) {
		resp, err := http.Get(srvURL)
		assert.NoError(it, err)
		assert.Equal(it, http.StatusTeapot, resp.StatusCode)
	})

	var done int32

	go func() {
		resp, err := http.Get(srvURL + "?wait=true")
		assert.NoError(t, err)
		assert.Equal(t, http.StatusTeapot, resp.StatusCode)
		atomic.AddInt32(&done, 1)
	}()

	eventually.Assert(t, func(it assert.It) { // wait until the long request is made
		assert.NotEqual(it, 0, atomic.LoadInt64(&inFlight))
	})

	cancel()

	// after cancellation, server still in running and handling last open connections
	for i := 0; i < 5; i++ {
		assert.Equal(t, 0, atomic.LoadInt32(&done))
		time.Sleep(time.Millisecond)
	}

	// then we travel forward in time, so the long request finish up its business
	timecop.Travel(t, time.Minute)

	eventually.Assert(t, func(it assert.It) {
		_, err := http.Get(srvURL)
		assert.Error(it, err)
	})
}

func TestHTTPServerPortFromENV(t *testing.T) {
	testcase.SetEnv(t, "PORT", "58080")
	const srvURL = "http://localhost:58080/"

	srv := &http.Server{
		Addr: "localhost:8080",
		Handler: http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			writer.WriteHeader(http.StatusTeapot)
		}),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() { assert.NoError(t, tasker.HTTPServerTask(srv, tasker.HTTPServerPortFromENV()).Run(ctx)) }()

	eventually := assert.MakeRetry(5 * time.Second)

	eventually.Assert(t, func(it assert.It) {
		resp, err := http.Get(srvURL)
		assert.NoError(it, err)
		assert.Equal(it, http.StatusTeapot, resp.StatusCode)
	})
}

func TestHTTPServerPortFromENV_replacePortInBindingAddress(t *testing.T) {
	testcase.SetEnv(t, "PORT", "58080")
	const srvURL = "http://127.0.0.1:58080/"

	srv := &http.Server{
		Addr: "127.0.0.1:8080",
		Handler: http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			writer.WriteHeader(http.StatusTeapot)
		}),
	}

	ctx := context.Background()

	join := tasker.Background(ctx, tasker.HTTPServerTask(srv, tasker.HTTPServerPortFromENV()))
	defer func() { assert.NoError(t, join()) }()

	eventually := assert.MakeRetry(5 * time.Second)

	eventually.Assert(t, func(it assert.It) {
		resp, err := http.Get(srvURL)
		assert.NoError(it, err)
		assert.Equal(it, http.StatusTeapot, resp.StatusCode)
	})
}

func TestHTTPServerPortFromENV_multiplePORTEnvVariable(t *testing.T) {
	testcase.SetEnv(t, "PORT", "58080")
	const srvURL = "http://127.0.0.1:58080/"

	srv := &http.Server{
		Addr: ":8080",
		Handler: http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			writer.WriteHeader(http.StatusTeapot)
		}),
	}

	ctx := context.Background()

	join := tasker.Background(ctx, tasker.HTTPServerTask(srv, tasker.HTTPServerPortFromENV()))
	defer func() { assert.NoError(t, join()) }()

	eventually := assert.MakeRetry(5 * time.Second)

	eventually.Assert(t, func(it assert.It) {
		resp, err := http.Get(srvURL)
		assert.NoError(it, err)
		assert.Equal(it, http.StatusTeapot, resp.StatusCode)
	})
}

func TestHTTPServerPortFromENV_httpServerAddrHasOnlyPort(t *testing.T) {
	testcase.UnsetEnv(t, "PORT2")
	testcase.SetEnv(t, "PORT", "58080")
	const srvURL = "http://127.0.0.1:58080/"

	srv := &http.Server{
		Addr: ":8080",
		Handler: http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			writer.WriteHeader(http.StatusTeapot)
		}),
	}

	ctx := context.Background()

	join := tasker.Background(ctx, tasker.HTTPServerTask(srv, tasker.HTTPServerPortFromENV("PORT2", "PORT")))
	defer func() { assert.NoError(t, join()) }()

	eventually := assert.MakeRetry(5 * time.Second)

	eventually.Assert(t, func(it assert.It) {
		resp, err := http.Get(srvURL)
		assert.NoError(it, err)
		assert.Equal(it, http.StatusTeapot, resp.StatusCode)
	})
}

func TestHTTPServerTask_withContextValuesPassedDownToRequests(t *testing.T) {
	const srvURL = "http://localhost:58080/"
	type ctxKey struct{}

	srv := &http.Server{
		Addr: "localhost:58080",
		Handler: http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			val, ok := request.Context().Value(ctxKey{}).(string)
			assert.True(t, ok)
			assert.Equal(t, "foo", val)
			writer.WriteHeader(http.StatusTeapot)
		}),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctx = context.WithValue(ctx, ctxKey{}, "foo")

	go func() { assert.NoError(t, tasker.HTTPServerTask(srv).Run(ctx)) }()

	assert.Eventually(t, time.Second, func(it assert.It) {
		resp, err := http.Get(srvURL)
		assert.NoError(it, err)
		assert.Equal(it, http.StatusTeapot, resp.StatusCode)
	})
}

func TestHTTPServerTask_requestContextIsNotDoneWhenAppContextIsCancelled(t *testing.T) {
	var (
		ready  int32
		done   int32
		finish = make(chan struct{})
	)

	srv := &http.Server{
		Addr: "localhost:58080",
		Handler: http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			atomic.AddInt32(&ready, 1)
			defer atomic.AddInt32(&done, 1)
			select {
			case <-request.Context().Done():
				t.Log("was not expected that this context gets cancelled due to the shutdown")
				t.Fail()
			case <-finish:
				// OK
			}
			writer.WriteHeader(http.StatusTeapot)
		}),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() { assert.NoError(t, tasker.HTTPServerTask(srv).Run(ctx)) }()

	go func() {
		resp, err := http.Get("http://localhost:58080/")
		assert.Should(t).NoError(err)
		assert.Should(t).Equal(http.StatusTeapot, resp.StatusCode)
	}()

	assert.Eventually(t, time.Second, func(it assert.It) {
		assert.Equal(it, atomic.LoadInt32(&ready), 1)
	})

	cancel()

	assert.NotWithin(t, time.Second/2, func(ctx context.Context) {
		for atomic.LoadInt32(&done) == 0 || ctx.Err() != nil {
		}
	})

	assert.NotEqual(t, atomic.LoadInt32(&done), 1)

	close(finish)

	assert.Eventually(t, time.Second, func(it assert.It) {
		assert.NotEqual(t, atomic.LoadInt32(&done), 1)
	})
}

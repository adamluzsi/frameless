package tasker

import (
	"context"
	"fmt"
	"go.llib.dev/frameless/pkg/contextkit"
	"go.llib.dev/frameless/pkg/env"
	"net"
	"net/http"
	"net/url"
	"strings"
)

// HTTPServerTask is designed to encapsulate your `http.Server`,
// enabling graceful shutdown with the server and presenting it as a Task.
func HTTPServerTask(srv *http.Server, opts ...httpServerTaskOption) Task {
	return WithShutdown(
		func(ctx context.Context) error {
			for _, opt := range opts {
				if err := opt.configureHTTPServer(srv); err != nil {
					return err
				}
			}
			if srv.BaseContext == nil {
				defer func() { srv.BaseContext = nil }()
				baseContext := contextkit.Detach(ctx)
				srv.BaseContext = func(net.Listener) context.Context {
					return baseContext
				}
			}
			return IgnoreError(
				srv.ListenAndServe,
				http.ErrServerClosed,
			).Run(ctx)
		},
		srv.Shutdown,
	)
}

type httpServerTaskOption interface {
	configureHTTPServer(*http.Server) error
}

type httpServerTaskOptionFunc func(*http.Server) error

func (fn httpServerTaskOptionFunc) configureHTTPServer(srv *http.Server) error { return fn(srv) }

func HTTPServerPortFromENV(envKeys ...string) httpServerTaskOption {
	if len(envKeys) == 0 {
		envKeys = append(envKeys, "PORT") // Default ENV key by convention
	}
	return httpServerTaskOptionFunc(func(server *http.Server) error {
		var (
			port int
			ok   bool
		)
		for _, key := range envKeys {
			var err error
			port, ok, err = env.Lookup[int](key)
			if err != nil {
				return fmt.Errorf("error while parsing port env value (%s): %w", key, err)
			}
			if ok {
				break
			}
		}
		if !ok {
			return fmt.Errorf("port environment variable is missing (%s)", strings.Join(envKeys, ", "))
		}
		u, err := url.Parse(server.Addr)
		if err != nil {
			return fmt.Errorf("error while parsing the server addr: %s\n%w", server.Addr, err)
		}
		server.Addr = fmt.Sprintf("%s:%d", u.Hostname(), port)
		return nil
	})
}

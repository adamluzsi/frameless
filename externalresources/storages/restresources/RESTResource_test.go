package restresources_test

import (
	"fmt"
	"github.com/adamluzsi/frameless/externalresources/storages/restresources"
	"github.com/adamluzsi/frameless/queries"
	"net"
	"net/http"
	"regexp"
	"testing"
	"time"
)

func TestRESTResource(t *testing.T) {
	t.Skip()
	t.Run("RESTful API as storage backend", func(t *testing.T) {

		m := NewMockService(t)
		defer m.Close()

		e := restresources.NewRESTResource(m.Addr())

		queries.TestAll(t, e, func(){})
	})
}

type MockService struct {
	server *http.Server
}

func (m *MockService) Addr() string {
	return m.server.Addr
}

func (m *MockService) Close() error {
	return m.server.Close()
}

func NewMockService(t testing.TB) *MockService {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(writer http.ResponseWriter, r *http.Request) {
		switch {
		case "/check" == r.URL.Path:
			fmt.Fprint(writer, "OK")
		case regexp.MustCompile(`/name`).MatchString(r.URL.Path):

			return
		}
	})

	port := freePort(t)

	server := &http.Server{Addr: fmt.Sprintf(":%d", port), Handler: mux}

	go func() {
		if err := server.ListenAndServe(); err != nil {
			t.Fatal(err)
		}
	}()

	checkAddr := fmt.Sprintf("http://localhost:%d/check", port)
	client := &http.Client{Timeout: time.Second}

	for {
		if _, err := client.Get(checkAddr); err == nil {
			break
		}
		time.Sleep(time.Millisecond)
	}

	return &MockService{server: server}
}

// freePort asks the kernel for a free open port that is ready to use.
func freePort(tb testing.TB) int {
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		tb.Fatal(err)
	}

	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		tb.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

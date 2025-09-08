package netkit_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"go.llib.dev/frameless/pkg/netkit"
	"go.llib.dev/frameless/pkg/synckit"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
)

const localhost = "127.0.0.1"

const allInterfaces = "0.0.0.0"

func TestIsPortFree_tcp(t *testing.T) {
	const port = 18881 // Choose a port that is likely to be free.

	t.Run("when port is in use", func(t *testing.T) {
		srv := &http.Server{
			Addr: fmt.Sprintf("%s:%d", localhost, port),
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusTeapot)
			}),
		}
		addr := HTTPServerServe(t, srv, "tcp")

		assert.Eventually(t, 5*time.Second, func(it testing.TB) {
			c := http.Client{Timeout: time.Second}
			resp, err := c.Get(fmt.Sprintf("http://%s", addr))
			assert.NoError(it, err)
			assert.Equal(it, resp.StatusCode, http.StatusTeapot)
		})

		t.Run("tcp", func(t *testing.T) {
			isPortOpen, err := netkit.IsPortFree("tcp", port)
			assert.NoError(t, err)
			assert.False(t, isPortOpen)
		})

		t.Run("udp", func(t *testing.T) {
			isPortOpen, err := netkit.IsPortFree("udp", port)
			assert.NoError(t, err)
			assert.True(t, isPortOpen)
		})

		t.Run("any as empty network id", func(t *testing.T) {
			isPortOpen, err := netkit.IsPortFree("", port)
			assert.NoError(t, err)
			assert.False(t, isPortOpen)
		})

		t.Run("any as *", func(t *testing.T) {
			isPortOpen, err := netkit.IsPortFree("*", port)
			assert.NoError(t, err)
			assert.False(t, isPortOpen)
		})
	})

	t.Run("when network type is invalid", func(t *testing.T) {
		_, err := netkit.IsPortFree("nan", port)
		assert.Error(t, err)
	})

	t.Run("when port is available", func(t *testing.T) {
		isOpen, err := netkit.IsPortFree("tcp", port)
		assert.NoError(t, err)
		assert.True(t, isOpen)
	})
}

// TestCheckPort tests the IsPortFree function with different scenarios.
func TestIsPortFree_udp(t *testing.T) {
	const port = 18881 // Choose a port that is likely to be free.

	t.Run("when port is in use", func(t *testing.T) {
		ip := net.ParseIP(localhost)
		c, err := net.ListenUDP("udp", &net.UDPAddr{
			IP:   ip,
			Port: port,
		})
		assert.NoError(t, err)
		defer c.Close()

		assert.Eventually(t, 5*time.Second, func(it testing.TB) {
			dial, err := net.Dial("udp", fmt.Sprintf("%s:%d", localhost, port))
			assert.NoError(it, err)
			assert.NoError(it, dial.Close())
		})

		isPortOpen, err := netkit.IsPortFree("tcp", port)
		assert.NoError(t, err)
		assert.True(t, isPortOpen)

		isPortOpen, err = netkit.IsPortFree("", port)
		assert.NoError(t, err)
		assert.False(t, isPortOpen)

		isPortOpen, err = netkit.IsPortFree("udp", port)
		assert.NoError(t, err)
		assert.False(t, isPortOpen)
	})

	t.Run("when port is available", func(t *testing.T) {
		isOpen, err := netkit.IsPortFree("udp", port)
		assert.NoError(t, err)
		assert.True(t, isOpen)
	})
}

func TestFreePort(t *testing.T) {
	var newHTTPClient = func(network netkit.Network) *http.Client {
		dialer := &net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}
		// Custom transport with Network
		transport := &http.Transport{
			DialContext: func(ctx context.Context, n, addr string) (net.Conn, error) {
				return dialer.DialContext(ctx, network.String(), addr)
			},
		}
		return &http.Client{
			Transport: transport,
			Timeout:   time.Second,
		}
	}

	for _, network := range []netkit.Network{netkit.TCP, netkit.TCP4, netkit.TCP6} {
		t.Run(fmt.Sprintf("it will return an open port for %s", network.String()), func(t *testing.T) {
			port, err := netkit.FreePort(network)
			assert.NoError(t, err)
			assert.NotEqual(t, port, 0)

			isFree, err := netkit.IsPortFree(network, port)
			assert.NoError(t, err)
			assert.True(t, isFree)

			srv := &http.Server{
				Addr: fmt.Sprintf(":%d", port),
				Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusTeapot)
				}),
			}

			addr := HTTPServerServe(t, srv, network)

			assert.Eventually(t, 5*time.Second, func(it testing.TB) {
				c := newHTTPClient(network)
				targetURL := fmt.Sprintf("http://%s", addr)
				resp, err := c.Get(targetURL)
				assert.NoError(it, err)
				assert.Equal(it, resp.StatusCode, http.StatusTeapot)
			})

			t.Run("and using the received port and requesting a new free port", func(t *testing.T) {
				nextPort, err := netkit.FreePort(network)
				assert.NoError(t, err)
				assert.NotEqual(t, nextPort, 0)
				isFree, err := netkit.IsPortFree(network, nextPort)
				assert.NoError(t, err)
				assert.True(t, isFree)
				assert.NotEqual(t, port, nextPort)
			})
		})
	}

	for _, network := range []netkit.Network{netkit.UDP, netkit.UDP4, netkit.UDP6} {
		t.Run(fmt.Sprintf("it will return an open port for %s", network.String()), func(t *testing.T) {
			port, err := netkit.FreePort(network)
			assert.NoError(t, err)
			assert.NotEqual(t, port, 0)

			isFree, err := netkit.IsPortFree(network, port)
			assert.NoError(t, err)
			assert.True(t, isFree)

			pc, err := net.ListenPacket(network.String(), fmt.Sprintf(":%d", port))
			assert.NoError(t, err)
			t.Cleanup(func() { _ = pc.Close() })

			isFree, err = netkit.IsPortFree(network, port)
			assert.NoError(t, err)
			assert.False(t, isFree)

			t.Run("and using the received port and requesting a new free port", func(t *testing.T) {
				nextPort, err := netkit.FreePort(network)
				assert.NoError(t, err)
				assert.NotEqual(t, nextPort, 0)
				isFree, err := netkit.IsPortFree(network, nextPort)
				assert.NoError(t, err)
				assert.True(t, isFree)
				assert.NotEqual(t, port, nextPort)
			})
		})
	}

	t.Run("works concurrently", func(t *testing.T) {
		assert.Eventually(t, 3*time.Second, func(t testing.TB) {
			var (
				got synckit.Map[int, struct{}]
				fns []func()
			)
			var networks = []netkit.Network{
				netkit.TCP, netkit.TCP4, netkit.TCP6,
				netkit.UDP, netkit.UDP4, netkit.UDP6,
			}
			for _, network := range networks {
				var network = network
				fns = append(fns, func() {
					port, err := netkit.FreePort(network)
					assert.Should(t).NoError(err)
					got.Set(port, struct{}{})
				})
			}
			testcase.Race(func() {}, func() {}, fns...)
			assert.True(t, 1 < got.Len())
		})
	})
}

func HTTPServerServe(tb testing.TB, srv *http.Server, n netkit.Network) string {
	tb.Helper()
	var addrChan = make(chan net.Addr)
	go func() {
		tb.Helper()
		tb.Logf("server address=%#v", srv.Addr)
		l, err := net.Listen(n.String(), srv.Addr)
		assert.NoError(tb, err)
		addrChan <- l.Addr()
		err = srv.Serve(l)
		if !errors.Is(err, http.ErrServerClosed) {
			assert.Should(tb).NoError(err)
		}
	}()
	tb.Cleanup(func() {
		tb.Helper()
		ctx := context.Background()
		err := srv.Shutdown(ctx)
		assert.Should(tb).NoError(err)
	})
	var netAddr = <-addrChan
	var addr = netAddr.String()
	const localhostAddressHost = "0.0.0.0"
	if strings.HasPrefix(addr, localhostAddressHost) {
		addr = strings.TrimPrefix(addr, localhostAddressHost)
		addr = localhost + addr
	}
	return addr
}

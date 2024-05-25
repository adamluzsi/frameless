package netkit_test

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	"go.llib.dev/frameless/pkg/netkit"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
)

func TestIsPortFree_tcp(t *testing.T) {
	const port = 18881 // Choose a port that is likely to be free.

	t.Run("when port is in use", func(t *testing.T) {
		srv := http.Server{
			Addr: fmt.Sprintf("0.0.0.0:%d", port),
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusTeapot)
			}),
		}
		go srv.ListenAndServe()
		defer srv.Shutdown(context.Background())

		assert.Eventually(t, 5*time.Second, func(it assert.It) {
			c := http.Client{Timeout: time.Second}
			resp, err := c.Get(fmt.Sprintf("http://%s", srv.Addr))
			it.Must.NoError(err)
			it.Must.Equal(resp.StatusCode, http.StatusTeapot)
		})

		isPortOpen, err := netkit.IsPortFree("tcp", port)
		assert.NoError(t, err)
		assert.False(t, isPortOpen)

		isPortOpen, err = netkit.IsPortFree("", port)
		assert.NoError(t, err)
		assert.False(t, isPortOpen)

		isPortOpen, err = netkit.IsPortFree("udp", port)
		assert.NoError(t, err)
		assert.True(t, isPortOpen)
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
		ip := net.ParseIP("0.0.0.0")
		c, err := net.ListenUDP("udp", &net.UDPAddr{
			IP:   ip,
			Port: port,
		})
		assert.NoError(t, err)
		defer c.Close()

		assert.Eventually(t, 5*time.Second, func(it assert.It) {
			dial, err := net.Dial("udp", fmt.Sprintf("0.0.0.0:%d", port))
			it.Must.NoError(err)
			it.Must.NoError(dial.Close())
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

func TestGetFreePort(t *testing.T) {
	t.Run("it will return an open port", func(t *testing.T) {
		port, err := netkit.GetFreePort()
		assert.NoError(t, err)
		assert.NotEqual(t, port, 0)

		isFree, err := netkit.IsPortFree("tcp", port)
		assert.NoError(t, err)
		assert.True(t, isFree)

		srv := http.Server{
			Addr: fmt.Sprintf("0.0.0.0:%d", port),
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusTeapot)
			}),
		}
		go srv.ListenAndServe()
		defer srv.Shutdown(context.Background())

		assert.Eventually(t, 5*time.Second, func(it assert.It) {
			c := http.Client{Timeout: time.Second}
			resp, err := c.Get(fmt.Sprintf("http://%s", srv.Addr))
			it.Must.NoError(err)
			it.Must.Equal(resp.StatusCode, http.StatusTeapot)
		})

		t.Run("and using the received port and requesting a new free port", func(t *testing.T) {
			nextPort, err := netkit.GetFreePort()
			assert.NoError(t, err)
			assert.NotEqual(t, nextPort, 0)
			isFree, err := netkit.IsPortFree("tcp", nextPort)
			assert.NoError(t, err)
			assert.True(t, isFree)
			assert.NotEqual(t, port, nextPort)
		})
	})
	t.Run("works concurrently", func(t *testing.T) {
		assert.Eventually(t, time.Minute, func(t assert.It) {
			var a, b, c, d int
			testcase.Race(func() {
				var err error
				a, err = netkit.GetFreePort()
				assert.Should(t).NoError(err)
			}, func() {
				var err error
				b, err = netkit.GetFreePort()
				assert.Should(t).NoError(err)
			}, func() {
				var err error
				c, err = netkit.GetFreePort()
				assert.Should(t).NoError(err)
			}, func() {
				var err error
				d, err = netkit.GetFreePort()
				assert.Should(t).NoError(err)
			})
			res := make(map[int]struct{})
			res[a] = struct{}{}
			res[b] = struct{}{}
			res[c] = struct{}{}
			res[d] = struct{}{}
			assert.NotEqual(t, 1, len(res))
		})
	})
}

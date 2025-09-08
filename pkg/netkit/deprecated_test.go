package netkit_test

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"go.llib.dev/frameless/pkg/netkit"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
)

func TestGetFreePort(t *testing.T) {
	t.Run("it will return an open port", func(t *testing.T) {
		port, err := netkit.GetFreePort()
		assert.NoError(t, err)
		assert.NotEqual(t, port, 0)

		isFree, err := netkit.IsPortFree("tcp", port)
		assert.NoError(t, err)
		assert.True(t, isFree)

		srv := &http.Server{
			Addr: fmt.Sprintf("%s:%d", localhost, port),
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusTeapot)
			}),
		}

		HTTPServerServe(t, srv, "tcp")

		assert.Eventually(t, 5*time.Second, func(it testing.TB) {
			c := http.Client{Timeout: time.Second}
			resp, err := c.Get(fmt.Sprintf("http://%s", srv.Addr))
			assert.NoError(it, err)
			assert.Equal(it, resp.StatusCode, http.StatusTeapot)
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
		assert.Eventually(t, time.Minute, func(t testing.TB) {
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

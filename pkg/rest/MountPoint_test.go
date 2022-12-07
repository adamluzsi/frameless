package rest_test

import (
	"fmt"
	"github.com/adamluzsi/frameless/pkg/rest"
	"github.com/adamluzsi/frameless/pkg/rest/internal"
	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/random"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMountPoint(tt *testing.T) {
	s := testcase.NewSpec(tt)
	t := testcase.NewT(tt, s)

	charset := random.CharsetDigit()
	mountPoint := rest.Path(fmt.Sprintf("/%s/%s/%s",
		t.Random.StringNC(3, charset),
		t.Random.StringNC(3, charset),
		t.Random.StringNC(3, charset),
	))
	var gotReq *http.Request
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotReq = r
		w.WriteHeader(http.StatusTeapot)
	})

	gotHandler := rest.MountPoint(mountPoint, handler)

	remainingPath := "/foo/bar/baz"
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, string(mountPoint)+remainingPath, nil)
	gotHandler.ServeHTTP(rr, req)
	t.Must.Equal(http.StatusTeapot, rr.Code)
	t.Must.NotNil(gotReq)
	rc, ok := internal.LookupRouting(gotReq.Context())
	t.Must.True(ok)
	t.Must.Equal(remainingPath, rc.Path)
}

package adapters_test

import (
	"bytes"
	"errors"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/julienschmidt/httprouter"
	"github.com/stretchr/testify/require"

	"github.com/adamluzsi/frameless/controllers/adapters"
)

func TestAdapterHTTPRouter_RequestBodyCanBeReadAsTheData_RequestSucceed(t *testing.T) {
	t.Parallel()

	mw := adapters.HTTPRouter(ControllerFor(t, nil, true, nil), MockPresenterBuilder(), MockIteratorBuilder())

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", bytes.NewReader([]byte(`Hello, World!`)))
	p := httprouter.Params{}

	mw(w, r, p)
	require.Equal(t, `Hello, World!`, w.Body.String())

}

func TestAdapterHTTPRouter_RequestBodyIsClosedAfterHandle_RequestSucceed(t *testing.T) {
	t.Parallel()

	mw := adapters.HTTPRouter(ControllerFor(t, nil, true, nil), MockPresenterBuilder(), MockIteratorBuilder())

	var body io.ReadCloser = &Body{Buffer: bytes.NewBuffer([]byte(`Hello, World!`))}
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", body)
	p := httprouter.Params{}

	mw(w, r, p)

	require.True(t, body.(*Body).IsClosed)
}

func TestAdapterHTTPRouter_QueryStringPresentInPath_RequestSucceed(t *testing.T) {
	t.Parallel()

	mw := adapters.HTTPRouter(ControllerFor(t, map[interface{}]interface{}{"k": "v"}, false, nil), MockPresenterBuilder(), MockIteratorBuilder())

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/?k=v", bytes.NewReader([]byte{}))
	p := httprouter.Params{httprouter.Param{Key: "k", Value: "v"}}

	mw(w, r, p)

	require.Equal(t, `v`, w.Body.String())
}

func TestAdapterHTTPRouter__RequestSucceed(t *testing.T) {
	t.Parallel()

	err := errors.New("KaBuM!")
	mw := adapters.HTTPRouter(ControllerFor(t, nil, false, err), MockPresenterBuilder(), MockIteratorBuilder())

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", bytes.NewReader([]byte{}))
	p := httprouter.Params{}

	mw(w, r, p)

	require.Equal(t, 500, w.Code)
	require.Equal(t, err.Error(), strings.TrimSpace(w.Body.String()))
}

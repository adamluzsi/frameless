package adapters_test

import (
	"bytes"
	"errors"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/adamluzsi/frameless/adapters"
	"github.com/stretchr/testify/require"
)

func TestAdapterNetHTTP_RequestBodyCanBeReadAsTheData_RequestSucceed(t *testing.T) {
	t.Parallel()

	mw := adapters.NetHTTP(ControllerFor(t, nil, true, nil), MockPresenterBuilder(), MockIteratorBuilder())

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", bytes.NewReader([]byte(`Hello, World!`)))

	mw.ServeHTTP(w, r)
	require.Equal(t, `Hello, World!`, w.Body.String())

}

func TestAdapterNetHTTP_RequestBodyIsClosedAfterHandle_RequestSucceed(t *testing.T) {
	t.Parallel()

	mw := adapters.NetHTTP(ControllerFor(t, nil, true, nil), MockPresenterBuilder(), MockIteratorBuilder())

	var body io.ReadCloser = &Body{Buffer: bytes.NewBuffer([]byte(`Hello, World!`))}
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", body)

	mw.ServeHTTP(w, r)

	require.True(t, body.(*Body).IsClosed)
}

func TestAdapterNetHTTP_QueryStringPresentInPath_RequestSucceed(t *testing.T) {
	t.Parallel()

	mw := adapters.NetHTTP(ControllerFor(t, map[interface{}]interface{}{"k": "v"}, false, nil), MockPresenterBuilder(), MockIteratorBuilder())

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/?k=v", bytes.NewReader([]byte{}))

	mw.ServeHTTP(w, r)

	require.Equal(t, `v`, w.Body.String())
}

func TestAdapterNetHTTP__RequestSucceed(t *testing.T) {
	t.Parallel()

	err := errors.New("KaBuM!")

	mw := adapters.NetHTTP(ControllerFor(t, nil, false, err), MockPresenterBuilder(), MockIteratorBuilder())

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", bytes.NewReader([]byte{}))

	mw.ServeHTTP(w, r)

	require.Equal(t, 500, w.Code)
	require.Equal(t, err.Error(), strings.TrimSpace(w.Body.String()))
}

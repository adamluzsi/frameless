package adapters_test

import (
	"bytes"
	"errors"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/adamluzsi/frameless/adapters"
)

func TestAdapterNetHTTP_RequestBodyCanBeReadAsTheData_RequestSucceed(t *testing.T) {
	t.Parallel()

	adapter := adapters.New(MockPresenterBuilder(), MockIteratorBuilder())
	mw := adapter.NetHTTP(ControllerFor(t, nil, true, nil))

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", bytes.NewReader([]byte(`Hello, World!`)))

	mw.ServeHTTP(w, r)
	require.Equal(t, `Hello, World!`, w.Body.String())

}

type Body struct {
	*bytes.Buffer
	IsClosed bool
}

func (b *Body) Close() error {
	b.IsClosed = true
	return nil
}

func TestAdapterNetHTTP_RequestBodyIsClosedAfterHandle_RequestSucceed(t *testing.T) {
	t.Parallel()

	adapter := adapters.New(MockPresenterBuilder(), MockIteratorBuilder())
	mw := adapter.NetHTTP(ControllerFor(t, nil, true, nil))

	var body io.ReadCloser = &Body{Buffer: bytes.NewBuffer([]byte(`Hello, World!`))}
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", body)

	mw.ServeHTTP(w, r)

	require.True(t, body.(*Body).IsClosed)
}

func TestAdapterNetHTTP_QueryStringPresentInPath_RequestSucceed(t *testing.T) {
	t.Parallel()

	adapter := adapters.New(MockPresenterBuilder(), MockIteratorBuilder())
	mw := adapter.NetHTTP(ControllerFor(t, map[interface{}]interface{}{"k": []string{"v"}}, false, nil))

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/?k=v", bytes.NewReader([]byte{}))

	mw.ServeHTTP(w, r)

	require.Equal(t, `[v]`, w.Body.String())
}

func TestAdapterNetHTTP__RequestSucceed(t *testing.T) {
	t.Parallel()

	err := errors.New("KaBuM!")
	adapter := adapters.New(MockPresenterBuilder(), MockIteratorBuilder())
	mw := adapter.NetHTTP(ControllerFor(t, nil, false, err))

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", bytes.NewReader([]byte{}))

	mw.ServeHTTP(w, r)

	require.Equal(t, 500, w.Code)
	require.Equal(t, err.Error(), strings.TrimSpace(w.Body.String()))
}

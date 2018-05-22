package http_test

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/adamluzsi/frameless/imp/net/http"
	"github.com/adamluzsi/frameless/imp/net/http/payload"
	"github.com/adamluzsi/frameless/request"
	require "github.com/stretchr/testify/require"
)

var _ request.Request = http.NewRequest(httptest.NewRequest("GET", "/", strings.NewReader("")), payload.LineByLine)

func TestRequest_HTTPRequestConfiguredValueReturned_QueryParametersTurnedIntoOptions(t *testing.T) {
	t.Parallel()

	httpRequest := httptest.NewRequest("GET", "/test?k=v", strings.NewReader("Hello, World!\nHow are you?"))
	frequest := http.NewRequest(httpRequest, payload.LineByLine)
	v, found := frequest.Options().Get("k")

	require.True(t, found)
	require.Equal(t, []string{"v"}, v.([]string))
}

func TestRequest_HTTPRequestConfiguredValueReturned_PayloadIterable(t *testing.T) {
	t.Parallel()

	httpRequest := httptest.NewRequest("GET", "/test?k=v", strings.NewReader("Hello, World!\nHow are you?"))
	frequest := http.NewRequest(httpRequest, payload.LineByLine)
	i := frequest.Data()

	var s string

	require.True(t, i.More())
	require.Nil(t, i.Decode(&s))
	require.Equal(t, "Hello, World!", s)

	require.True(t, i.More())
	require.Nil(t, i.Decode(&s))
	require.Equal(t, "How are you?", s)

	require.False(t, i.More())
}

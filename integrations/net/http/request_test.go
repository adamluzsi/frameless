package http_test

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/adamluzsi/frameless"
	fhttp "github.com/adamluzsi/frameless/integrations/net/http"
	"github.com/adamluzsi/frameless/iterateover"
	require "github.com/stretchr/testify/require"
)

var _ frameless.Request = fhttp.NewRequest(nil, nil)

func TestRequestOptionsLookup_HTTPRequestConfiguredValueReturned_QueryParametersTurnedIntoOptions(t *testing.T) {
	t.Parallel()

	httpRequest := httptest.NewRequest("GET", "/test?k=v", strings.NewReader("Hello, World!\nHow are you?"))
	frequest := fhttp.NewRequest(httpRequest, iterateover.LineByLine)
	v, found := frequest.Options().Lookup("k")

	require.True(t, found)
	require.Equal(t, "v", v.(string))
}

func TestRequestOptionsGet_HTTPRequestConfiguredValueReturned_QueryParametersTurnedIntoOptions(t *testing.T) {
	t.Parallel()

	httpRequest := httptest.NewRequest("GET", "/test?k=v", strings.NewReader("Hello, World!\nHow are you?"))
	frequest := fhttp.NewRequest(httpRequest, iterateover.LineByLine)
	v := frequest.Options().Get("k")

	require.NotNil(t, v)
	require.Equal(t, "v", v.(string))
}

func TestRequestData_HTTPRequestConfiguredValueReturned_PayloadIterable(t *testing.T) {
	t.Parallel()

	httpRequest := httptest.NewRequest("GET", "/test?k=v", strings.NewReader("Hello, World!\nHow are you?"))
	frequest := fhttp.NewRequest(httpRequest, iterateover.LineByLine)
	i := frequest.Data()

	var s string

	require.True(t, i.Next())
	require.Nil(t, i.Decode(&s))
	require.Equal(t, "Hello, World!", s)

	require.True(t, i.Next())
	require.Nil(t, i.Decode(&s))
	require.Equal(t, "How are you?", s)

	require.False(t, i.Next())
}

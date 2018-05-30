package httprouter_test

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/adamluzsi/frameless"
	fhttprouter "github.com/adamluzsi/frameless/integrations/github.com/julienschmidt/httprouter"
	httprouter "github.com/julienschmidt/httprouter"

	"github.com/adamluzsi/frameless/iterate"
	require "github.com/stretchr/testify/require"
)

var _ frameless.Request = fhttprouter.NewRequest(nil, nil, nil)

func TestRequestOptionsLookup_HTTPRequestConfiguredValueReturned_QueryParametersTurnedIntoOptions(t *testing.T) {
	t.Parallel()

	httpRequest := httptest.NewRequest("GET", "/test?k=v", strings.NewReader("Hello, World!\nHow are you?"))
	frequest := fhttprouter.NewRequest(httpRequest, iterate.LineByLine, httprouter.Params{httprouter.Param{Key: "k", Value: "v"}})
	v, found := frequest.Options().Lookup("k")

	require.True(t, found)
	require.Equal(t, "v", v.(string))
}

func TestRequestOptionsGet_HTTPRequestConfiguredValueReturned_QueryParametersTurnedIntoOptions(t *testing.T) {
	t.Parallel()

	httpRequest := httptest.NewRequest("GET", "/test?k=v", strings.NewReader("Hello, World!\nHow are you?"))
	frequest := fhttprouter.NewRequest(httpRequest, iterate.LineByLine, httprouter.Params{httprouter.Param{Key: "k", Value: "v"}})
	v := frequest.Options().Get("k")

	require.NotNil(t, v)
	require.Equal(t, "v", v.(string))
}

func TestRequestData_HTTPRequestConfiguredValueReturned_PayloadIterable(t *testing.T) {
	t.Parallel()

	httpRequest := httptest.NewRequest("GET", "/test?k=v", strings.NewReader("Hello, World!\nHow are you?"))
	frequest := fhttprouter.NewRequest(httpRequest, iterate.LineByLine, httprouter.Params{})
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

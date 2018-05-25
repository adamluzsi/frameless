package httprouter_test

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/adamluzsi/frameless/dataproviders"
	fhttprouter "github.com/adamluzsi/frameless/integrations/github.com/julienschmidt/httprouter"
	httprouter "github.com/julienschmidt/httprouter"

	"github.com/adamluzsi/frameless/iterate"
	"github.com/adamluzsi/frameless/requests"
	require "github.com/stretchr/testify/require"
)

var _ requests.Request = fhttprouter.NewRequest(nil, nil, nil)

func TestRequestOptionsMultiGetter(t *testing.T) {
	t.Parallel()

	httpRequest := httptest.NewRequest("GET", "/test?k=v&k=c", strings.NewReader("Hello, World!\nHow are you?"))
	frequest := fhttprouter.NewRequest(httpRequest, iterate.LineByLine, httprouter.Params{httprouter.Param{Key: "k", Value: "v"}, httprouter.Param{Key: "k", Value: "c"}})
	mgetter := frequest.Options().(dataproviders.MultiGetter)

	vs := mgetter.GetAll("k")
	require.Equal(t, 2, len(vs))
	require.Equal(t, "v", vs[0])
	require.Equal(t, "c", vs[1])

	vs, ok := mgetter.LookupAll("k")
	require.Equal(t, true, ok)
	require.Equal(t, 2, len(vs))
	require.Equal(t, "v", vs[0])
	require.Equal(t, "c", vs[1])
}

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

	require.True(t, i.More())
	require.Nil(t, i.Decode(&s))
	require.Equal(t, "Hello, World!", s)

	require.True(t, i.More())
	require.Nil(t, i.Decode(&s))
	require.Equal(t, "How are you?", s)

	require.False(t, i.More())
}

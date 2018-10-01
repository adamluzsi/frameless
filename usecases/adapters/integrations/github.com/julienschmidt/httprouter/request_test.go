package httprouter_test

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/adamluzsi/frameless"
	fhttprouter "github.com/adamluzsi/frameless/usecases/adapters/integrations/github.com/julienschmidt/httprouter"
	"github.com/adamluzsi/frameless/iterators/iterateover"
	httprouter "github.com/julienschmidt/httprouter"

	require "github.com/stretchr/testify/require"
)

var _ frameless.Request = fhttprouter.NewRequest(
	httptest.NewRequest("GET", "/test?k=v", strings.NewReader("github.com/julienschmidt/httprouter")),
	iterateover.LineByLine,
	httprouter.Params{},
)

func TestRequestContextValue_HTTPRequestConfiguredValueReturned_QueryParametersTurnedIntoOptions(t *testing.T) {
	t.Parallel()

	httpRequest := httptest.NewRequest("GET", "/test?k=v", strings.NewReader("Hello, World!\nHow are you?"))
	frequest := fhttprouter.NewRequest(httpRequest, iterateover.LineByLine, httprouter.Params{httprouter.Param{Key: "k", Value: "v"}})
	v := frequest.Context().Value("k")

	require.NotNil(t, v)
	require.Equal(t, "v", v.(string))
}

func TestRequestData_HTTPRequestConfiguredValueReturned_PayloadIterable(t *testing.T) {
	t.Parallel()

	httpRequest := httptest.NewRequest("GET", "/test?k=v", strings.NewReader("Hello, World!\nHow are you?"))
	frequest := fhttprouter.NewRequest(httpRequest, iterateover.LineByLine, httprouter.Params{})
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

package http_test

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/adamluzsi/frameless"
	fhttp "github.com/adamluzsi/frameless/controllers/adapters/integrations/net/http"
	"github.com/adamluzsi/frameless/iterators/iterateover"
	require "github.com/stretchr/testify/require"
)

var _ frameless.Request = fhttp.NewRequest(
	httptest.NewRequest("GET", "/test", strings.NewReader("Hello, World!")),
	iterateover.LineByLine,
)

func TestRequestContextValue_HTTPRequestConfiguredValueReturned_QueryParametersTurnedIntoOptions(t *testing.T) {
	t.Parallel()

	httpRequest := httptest.NewRequest("GET", "/test?k=v", strings.NewReader(""))
	frequest := fhttp.NewRequest(httpRequest, iterateover.LineByLine)
	v := frequest.Context().Value("k")

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

func TestNewRequestWithContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())

	httpRequest := httptest.NewRequest("GET", "/test?k=v", strings.NewReader("Hello, World!\nHow are you?"))
	frequest := fhttp.NewRequestWithContext(ctx, httpRequest, iterateover.LineByLine)

	cancel()

	select {
	case <-frequest.Context().Done():
	case <-time.After(5 * time.Second):
		t.Fatal("context was not cancelled and test timed out")
	}

	// context should not be manipulated if it is provided from external source
	require.Nil(t, frequest.Context().Value("k"))
}

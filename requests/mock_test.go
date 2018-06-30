package requests_test

import (
	"context"
	"errors"
	"testing"

	"github.com/adamluzsi/frameless/iterators"
	"github.com/stretchr/testify/require"

	"github.com/adamluzsi/frameless/requests"
)

func TestMock(t *testing.T) {
	t.Parallel()

	m := requests.NewMock(context.Background(), iterators.NewSingleElement("Hello, World!"))
	i := m.Data()

	var value string
	require.Nil(t, iterators.DecodeNext(i, &value))
	require.Equal(t, "Hello, World!", value)

	require.Nil(t, i.Close())
	require.Error(t, errors.New("closed"), iterators.DecodeNext(i, &value))
}

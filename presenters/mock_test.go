package presenters_test

import (
	"errors"
	"testing"

	"github.com/adamluzsi/frameless"
	"github.com/stretchr/testify/require"

	"github.com/adamluzsi/frameless/presenters"
)

var _ frameless.Presenter = &presenters.Mock{}

func TestMock(t *testing.T) {
	t.Parallel()

	err := errors.New("Boom!")
	msg := "OK"

	m := &presenters.Mock{ReturnError: err}

	require.Equal(t, err, m.Render(msg))
	require.Equal(t, msg, m.LastReceivedMessage())
}

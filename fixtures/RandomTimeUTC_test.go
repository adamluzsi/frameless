package fixtures_test

import (
	"github.com/adamluzsi/frameless/fixtures"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestRandomTimeUTC(t *testing.T) {
	subject := func() time.Time { return fixtures.RandomTimeUTC() }

	loc, err := time.LoadLocation(`UTC`)
	require.Nil(t, err)
	require.Equal(t, loc, subject().Location(), `location should be in UTC`)
	require.False(t, subject().IsZero(), `results should be non zero time value`)
	require.NotEqual(t, subject(), subject(), `two separate run should create distinct values`)
}

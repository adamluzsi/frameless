package interfaces_test

import (
	"testing"

	"github.com/adamluzsi/frameless/interfaces"
	"github.com/stretchr/testify/require"
)

type TestStruct struct {
	TEXT string
}

func TestReplaceValue_PointerTypeGiven_LinkDone(t *testing.T) {
	t.Parallel()

	ts1 := &TestStruct{TEXT: "A"}
	ts2 := &TestStruct{TEXT: "B"}

	interfaces.ReplaceValue(ts1, ts2)

	require.Equal(t, ts1, ts2)
}

func TestReplaceValue_SimpleTypeGiven_LinkDone(t *testing.T) {
	t.Parallel()

	ts1 := TestStruct{TEXT: "A"}
	ts2 := &TestStruct{TEXT: "B"}

	interfaces.ReplaceValue(ts1, ts2)

	require.Equal(t, ts1, *ts2)
}

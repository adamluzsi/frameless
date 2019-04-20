package reflects_test

import (
	"github.com/adamluzsi/frameless/reflects"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestNew(t *testing.T) {
	t.Parallel()

	var inputAsType interface{}

	t.Run(`when struct given as type reference`, func(t *testing.T) {
		inputAsType = StructObject{}

		t.Run(`then it will return the a pointer to the base type`, func(t *testing.T) {
			require.Equal(t, &StructObject{}, reflects.New(inputAsType))
		})
	})

	t.Run(`when pointer to a struct given as type reference`, func(t *testing.T) {
		inputAsType = &StructObject{}

		t.Run(`then it will return the a pointer to the base type`, func(t *testing.T) {
			require.Equal(t, &StructObject{}, reflects.New(inputAsType))
		})
	})

	t.Run(`when pointer to a pointer which points to a struct given as type reference`, func(t *testing.T) {
		ptrToStruct := &StructObject{}
		ptrToPtr := &ptrToStruct
		inputAsType = ptrToPtr

		t.Run(`then it will return the a pointer to the base type`, func(t *testing.T) {
			require.Equal(t, &StructObject{}, reflects.New(inputAsType))
		})
	})
}

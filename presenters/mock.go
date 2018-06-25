package presenters

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

func NewMock() *Mock {
	return &Mock{}
}

type Mock struct {
	ReceivedMessages []interface{}
	ReturnError      error
}

func (m *Mock) Render(message interface{}) error {
	m.ReceivedMessages = append(m.ReceivedMessages, message)
	return m.ReturnError
}

func (m *Mock) LastReceivedMessage() interface{} {
	return m.ReceivedMessages[len(m.ReceivedMessages)-1]
}

func (m *Mock) MessageMatch(t testing.TB, i interface{}) {
	expected := reflect.ValueOf(i)
	actually := reflect.ValueOf(m.LastReceivedMessage())

	switch expected.Kind() {
	case reflect.Slice:
		require.Equal(t, expected.Len(), actually.Len())

		for index := 0; index < actually.Len(); index++ {
			require.Contains(t, i, actually.Index(index).Interface())
		}

	default:
		require.Equal(t, i, m.LastReceivedMessage())

	}
}

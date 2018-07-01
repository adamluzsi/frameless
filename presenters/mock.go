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

func (m *Mock) Message() interface{} {
	return m.ReceivedMessages[len(m.ReceivedMessages)-1]
}

func (m *Mock) MessageMatch(t testing.TB, i interface{}) {
	expected := reflect.ValueOf(i)
	actually := reflect.ValueOf(m.Message())

	switch expected.Kind() {
	case reflect.Slice:
		m.matchSlice(t, expected, actually)

	default:
		require.Equal(t, i, m.Message())

	}
}

func (m *Mock) StreamContains(t testing.TB, i interface{}) {
	expected := reflect.ValueOf(i)
	actually := reflect.ValueOf(m.ReceivedMessages)

	switch expected.Kind() {
	case reflect.Slice:
		m.matchSlice(t, expected, actually)

	default:
		require.Contains(t, m.ReceivedMessages, i)

	}
}

func (m *Mock) matchSlice(t testing.TB, expected, actually reflect.Value) {
	require.Equal(t, expected.Len(), actually.Len())

	for index := 0; index < actually.Len(); index++ {
		require.Contains(t, expected.Interface(), actually.Index(index).Interface())
	}
}

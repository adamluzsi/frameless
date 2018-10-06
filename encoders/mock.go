package encoders

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

func NewMock() *Mock {
	return &Mock{}
}

type Mock struct {
	Received    []interface{}
	ReturnError error
}

func (m *Mock) Encode(e interface{}) error {
	m.Received = append(m.Received, e)
	return m.ReturnError
}

func (m *Mock) Entity() interface{} {
	return m.Received[len(m.Received)-1]
}

func (m *Mock) MessageMatch(t testing.TB, i interface{}) {
	expected := reflect.ValueOf(i)
	actually := reflect.ValueOf(m.Entity())

	switch expected.Kind() {
	case reflect.Slice:
		m.matchSlice(t, expected, actually)

	default:
		require.Equal(t, i, m.Entity())

	}
}

func (m *Mock) StreamContains(t testing.TB, i interface{}) {
	expected := reflect.ValueOf(i)
	actually := reflect.ValueOf(m.Received)

	switch expected.Kind() {
	case reflect.Slice:
		m.matchSlice(t, expected, actually)

	default:
		require.Contains(t, m.Received, i)

	}
}

func (m *Mock) matchSlice(t testing.TB, expected, actually reflect.Value) {
	require.Equal(t, expected.Len(), actually.Len())

	for index := 0; index < actually.Len(); index++ {
		require.Contains(t, expected.Interface(), actually.Index(index).Interface())
	}
}

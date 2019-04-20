package reflects_test

import (
	"github.com/adamluzsi/frameless/reflects"
	"github.com/stretchr/testify/require"
	"reflect"
	"testing"
)

func TestBaseTypeOf(t *testing.T) {
	subject := func(obj interface{}) reflect.Type {
		return reflects.BaseTypeOf(obj)
	}

	SpecForPrimitiveNames(t, func(obj interface{}) string {
		return subject(obj).Name()
	})

	expectedValueType := reflect.TypeOf(StructObject{})

	plainStruct := StructObject{}
	ptrToStruct := &plainStruct
	ptrToPtr := &ptrToStruct

	require.Equal(t, expectedValueType, subject(plainStruct))
	require.Equal(t, expectedValueType, subject(ptrToStruct))
	require.Equal(t, expectedValueType, subject(ptrToPtr))
}

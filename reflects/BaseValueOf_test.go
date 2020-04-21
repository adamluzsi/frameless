package reflects_test

import (
	"reflect"
	"testing"

	"github.com/adamluzsi/frameless/reflects"
	"github.com/stretchr/testify/require"
)

func TestBaseValueOf(t *testing.T) {
	subject := func(input interface{}) reflect.Value {
		return reflects.BaseValueOf(input)
	}

	SpecForPrimitiveNames(t, func(obj interface{}) string {
		return subject(obj).Type().Name()
	})

	expectedValue := reflect.ValueOf(StructObject{})
	expectedValueType := expectedValue.Type()

	plainStruct := StructObject{}
	ptrToStruct := &plainStruct
	ptrToPtr := &ptrToStruct

	require.Equal(t, expectedValueType, subject(plainStruct).Type())
	require.Equal(t, expectedValueType, subject(ptrToStruct).Type())
	require.Equal(t, expectedValueType, subject(ptrToPtr).Type())
}

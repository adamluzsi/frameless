package reflects_test

import (
	"reflect"
	"testing"

	"github.com/adamluzsi/frameless/reflects"
	"github.com/adamluzsi/testcase/assert"
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

	assert.Must(t).Equal(expectedValueType, subject(plainStruct).Type())
	assert.Must(t).Equal(expectedValueType, subject(ptrToStruct).Type())
	assert.Must(t).Equal(expectedValueType, subject(ptrToPtr).Type())
}

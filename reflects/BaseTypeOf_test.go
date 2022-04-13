package reflects_test

import (
	"reflect"
	"testing"

	"github.com/adamluzsi/frameless/reflects"
	"github.com/adamluzsi/testcase/assert"
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

	assert.Must(t).Equal(expectedValueType, subject(plainStruct))
	assert.Must(t).Equal(expectedValueType, subject(ptrToStruct))
	assert.Must(t).Equal(expectedValueType, subject(ptrToPtr))
}

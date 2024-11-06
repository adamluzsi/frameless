package extid

import (
	"reflect"
	"testing"

	"go.llib.dev/frameless/port/crud/extid/internal/testhelper"
	"go.llib.dev/testcase/assert"
)

func TestLookupStructField(t *testing.T) {
	var (
		field reflect.StructField
		value reflect.Value
		ok    bool
	)

	field, value, ok = ExtractIdentifierField(testhelper.IDByIDField{ID: `42`})
	assert.True(t, ok)
	assert.Equal(t, `ID`, field.Name)
	assert.Equal(t, `42`, value.Interface())

	field, value, ok = ExtractIdentifierField(testhelper.IDByUppercaseTag{DI: `42`})
	assert.True(t, ok)
	assert.Equal(t, `DI`, field.Name)
	assert.Equal(t, `42`, value.Interface())

	field, value, ok = ExtractIdentifierField(testhelper.IDByLowercaseTag{DI: `42`})
	assert.True(t, ok)
	assert.Equal(t, `DI`, field.Name)
	assert.Equal(t, `42`, value.Interface())

	field, value, ok = ExtractIdentifierField(testhelper.IDAsInterface{ID: 42})
	assert.True(t, ok)
	assert.Equal(t, `ID`, field.Name)
	assert.Equal(t, 42, value.Interface())

	idValue := `42`
	field, value, ok = ExtractIdentifierField(testhelper.IDAsPointer{ID: &idValue})
	assert.True(t, ok)
	assert.Equal(t, `ID`, field.Name)
	assert.Equal(t, &idValue, value.Interface().(*string))

	_, _, ok = ExtractIdentifierField(testhelper.UnidentifiableID{})
	assert.Must(t).False(ok)
}

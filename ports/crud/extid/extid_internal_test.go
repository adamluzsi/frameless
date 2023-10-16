package extid

import (
	"go.llib.dev/frameless/ports/crud/extid/internal/testhelper"
	"github.com/adamluzsi/testcase/assert"
	"reflect"
	"testing"
)

func TestLookupStructField(t *testing.T) {
	var (
		field reflect.StructField
		value reflect.Value
		ok    bool
	)

	field, value, ok = lookupStructField(testhelper.IDByIDField{ID: `42`})
	assert.Must(t).True(ok)
	assert.Must(t).Equal(`ID`, field.Name)
	assert.Must(t).Equal(`42`, value.Interface())

	field, value, ok = lookupStructField(testhelper.IDByUppercaseTag{DI: `42`})
	assert.Must(t).True(ok)
	assert.Must(t).Equal(`DI`, field.Name)
	assert.Must(t).Equal(`42`, value.Interface())

	field, value, ok = lookupStructField(testhelper.IDByLowercaseTag{DI: `42`})
	assert.Must(t).True(ok)
	assert.Must(t).Equal(`DI`, field.Name)
	assert.Must(t).Equal(`42`, value.Interface())

	field, value, ok = lookupStructField(testhelper.IDAsInterface{ID: 42})
	assert.Must(t).True(ok)
	assert.Must(t).Equal(`ID`, field.Name)
	assert.Must(t).Equal(42, value.Interface())

	idValue := `42`
	field, value, ok = lookupStructField(testhelper.IDAsPointer{ID: &idValue})
	assert.Must(t).True(ok)
	assert.Must(t).Equal(`ID`, field.Name)
	assert.Must(t).Equal(&idValue, value.Interface())

	field, value, ok = lookupStructField(testhelper.UnidentifiableID{})
	assert.Must(t).False(ok)
}

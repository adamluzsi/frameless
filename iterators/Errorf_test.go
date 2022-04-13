package iterators_test

import (
	"testing"

	"github.com/adamluzsi/frameless/iterators"
	"github.com/adamluzsi/testcase/assert"
)

func TestErrorf(t *testing.T) {
	i := iterators.Errorf[any]("%s", "hello world!")
	assert.Must(t).NotNil(i)
	assert.Must(t).Equal("hello world!", i.Err().Error())
}

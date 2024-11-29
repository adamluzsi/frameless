package constraintkit_test

import (
	"testing"

	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/testcase/assert"
)

type MyType struct {
	V1 string   `len:"11"`
	V2 string   `len:"8-12"`
	V3 int      `min:"7" max:"42"`
	L1 []string `notnil:"true"`
	L2 []int    `notnil:"false" len:"3"`
}

func Test_spike(t *testing.T) {
	rt := reflectkit.TypeOf[MyType]()

	f, ok := rt.FieldByName("L1")
	assert.True(t, ok)
	t.Log(f.Tag)

	_, ok = f.Tag.Lookup("notnil")
	assert.Should(t).True(ok, "Expected that notnil tag can be found")

	v, ok := f.Tag.Lookup("len")
	assert.True(t, ok)
	t.Logf("%#v", v)
}

package dtom_test

import (
	"encoding/json"
	"github.com/adamluzsi/frameless/pkg/dtom"
	"github.com/adamluzsi/testcase/assert"
	"github.com/adamluzsi/testcase/pp"
	"github.com/adamluzsi/testcase/random"
	"testing"
)

func TestRegistry_smoke(t *testing.T) {
	var rnd = random.New(random.CryptoSeed{})

	ent := Foo{
		Bar: Bar{
			V: rnd.Int(),
		},
	}

	ogDTO, err := dtom.MapDTO(r, ent)
	assert.NoError(t, err)
	assert.NotEmpty(t, ogDTO)

	data, err := json.Marshal(ogDTO)
	assert.NoError(t, err)

	pp.PP(data)
	var resDTO dtom.Struct
	assert.NoError(t, json.Unmarshal(data, &resDTO))

	val := dtom.Must(dtom.MapValue[Foo](r, resDTO))

	pp.PP(val)
}

type Foo struct {
	Bar Bar
	Baz Baz
}

type Bar struct{ V int }

type Bazer interface{ Baz() }

type Baz struct{ V string }

func (Baz) Baz() {}

type Qux struct {
	Baz Bazer
}

type QuxDTO struct {
	Baz BazerDTO
}

type BazerDTO dtom.Interface[Bazer]

type FooDTO struct {
	Bar BarDTO `json:"bar"`
	Baz BazDTO `json:"baz"`
}

type BarDTO struct {
	V int `json:"v"`
}

type BazDTO struct {
	V string `json:"v"`
}

var r = &dtom.Registry{}

var _ = dtom.RegisterStruct[Foo, FooDTO](r, "foo", func(v Foo) (FooDTO, error) {
	return FooDTO{
		Bar: dtom.Must(dtom.MapDTO[BarDTO](r, v.Bar)),
	}, nil
}, func(dto FooDTO) (Foo, error) {
	return Foo{
		Bar: dtom.Must(dtom.MapValue[Bar](r, dto.Bar)),
		Baz: dtom.Must(dtom.MapValue[Baz](r, dto.Baz)),
	}, nil
})

var _ = dtom.RegisterStruct[Bar, BarDTO](r, "baz", func(v Bar) (BarDTO, error) {
	return BarDTO{V: v.V}, nil
}, func(dto BarDTO) (Bar, error) {
	return Bar{V: dto.V}, nil
})

var _ = dtom.RegisterStruct[Baz, BazDTO](r, "bar", func(v Baz) (BazDTO, error) {
	return BazDTO{V: v.V}, nil
}, func(dto BazDTO) (Baz, error) {
	return Baz{V: dto.V}, nil
})

var _ = dtom.RegisterInterface[Bazer](r, Baz{})

var _ = dtom.RegisterStruct[Qux, QuxDTO](r, "qux", func(v Baz) (BazDTO, error) {
	return BazDTO{V: v.V}, nil
}, func(dto QuxDTO) (Qux, error) {
	return Qux{
		Baz: dtom.Must[Baz](dtom.MapDTO[Baz](r, dto.Baz)),
	}, nil
})

package dtom_test

import (
	"github.com/adamluzsi/frameless/pkg/dtom"
	"github.com/adamluzsi/testcase/random"
	"testing"
)

type Foo struct {
	Bar    Bar
	Quxers []Quxer
}

type Bar struct {
	Baz Baz
}

type Baz struct {
	V int
}

type Quxer interface{ Qux() }

type Qux struct {
	V string
}

func (q Qux) Qux() {}

func TestRegistry_smoke(t *testing.T) {
	var rnd = random.New(random.CryptoSeed{})

	ent := Foo{
		Bar: Bar{
			Baz: Baz{
				V: rnd.Int(),
			},
		},
		Quxers: []Quxer{Qux{V: rnd.String()}},
	}

	r := &dtom.Registry{}

	dtom.RegisterStruct(r, dtom.StructMapping[Foo]{
		Check: func(str dtom.Struct) bool {
			return str["type"] == "foo"
		},
		ToEnt: func(str dtom.Struct) (Foo, error) {
			return Foo{
				Bar: dtom.MapVal[Bar](r, str.Object("bar")),
				Quxers: dtom.MapList(str.List("quxers"), func(v dtom.Struct) Quxer {
					return dtom.MapVal[Quxer](r, v)
				}),
			}, nil
		},
		ToDTO: func(ent Foo) (dtom.Struct, error) {
			return dtom.Struct{
				"bar":    dtom.MapDTO(r, ent.Bar),
				"quxers": dtom.MapDTO(r, ent.Quxers),
			}, nil
		},
	})

	dtom.RegisterStruct(r, dtom.StructMapping[Bar]{
		Check: func(str dtom.Struct) bool {
			return str["type"] == "bar"
		},
		ToEnt: func(dto dtom.Struct) (Bar, error) {
			return Bar{
				Baz: dtom.MapVal[Baz](r, dto.Object("baz")),
			}, nil
		},
		ToDTO: func(ent Bar) (dtom.Struct, error) {
			return dtom.Struct{
				"baz": dtom.MapDTO(r, ent.Baz),
			}, nil
		},
	})

	dtom.RegisterStruct(r, dtom.StructMapping[Qux]{
		Check: func(str dtom.Struct) bool {
			return str["type"] == "qux"
		},
		ToEnt: func(str dtom.Struct) (Qux, error) {
			return Qux{
				V: dtom.MapVal[string](r, str["v"]),
			}, nil
		},
		ToDTO: func(ent Qux) (dtom.Struct, error) {
			return dtom.Struct{
				"v": dtom.MapDTO(r, ent.V),
			}, nil
		},
	})
}

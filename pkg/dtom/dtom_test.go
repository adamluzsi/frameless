package dtom_test

import (
	"encoding/json"
	"github.com/adamluzsi/frameless/pkg/dtom"
	"github.com/adamluzsi/testcase/assert"
	"github.com/adamluzsi/testcase/pp"
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

var r = &dtom.Registry{}

var _ = dtom.RegisterStruct[Foo](r, "foo",
	func(str dtom.Struct) (Foo, error) {
		return Foo{
			Bar: dtom.Must(dtom.MapValue[Bar](r, str.Object("bar"))),
			Quxers: dtom.MapList(str.List("quxers"), func(v dtom.Struct) Quxer {
				return dtom.Must(dtom.MapValue[Quxer](r, v))
			}),
		}, nil
	},
	func(ent Foo) (dtom.Struct, error) {
		return dtom.Struct{
			"bar":    dtom.Must(dtom.MapDTO(r, ent.Bar)),
			"quxers": dtom.Must(dtom.MapDTO(r, ent.Quxers)),
		}, nil
	},
)

var _ = dtom.RegisterStruct[Bar](r, "baz",
	func(dto dtom.Struct) (Bar, error) {
		return Bar{
			Baz: dtom.Must(dtom.MapValue[Baz](r, dto.Object("baz"))),
		}, nil
	},
	func(ent Bar) (dtom.Struct, error) {
		return dtom.Struct{
			"baz": dtom.Must(dtom.MapDTO(r, ent.Baz)),
		}, nil
	},
)

var _ = dtom.RegisterStruct[Baz](r, "bar",
	func(dto dtom.Struct) (Baz, error) {
		return Baz{V: dtom.Must(dtom.MapValue[int](r, dto["v"]))}, nil
	},
	func(ent Baz) (dtom.Struct, error) {
		return dtom.Struct{"v": dtom.Must(dtom.MapDTO(r, ent.V))}, nil
	},
)

var _ = dtom.RegisterStruct[Qux](r, "qux",
	func(str dtom.Struct) (Qux, error) {
		return Qux{
			V: dtom.Must(dtom.MapValue[string](r, str["v"])),
		}, nil
	},
	func(ent Qux) (dtom.Struct, error) {
		return dtom.Struct{
			"v": dtom.Must(dtom.MapDTO(r, ent.V)),
		}, nil
	},
)

var _ = dtom.RegisterInterface[Quxer](r, Qux{})

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

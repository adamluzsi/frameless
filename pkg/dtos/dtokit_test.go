package dtos_test

import (
	"encoding/json"
	"go.llib.dev/frameless/pkg/dtos"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/random"
	"strconv"
	"testing"
)

var rnd = random.New(random.CryptoSeed{})

func TestM(t *testing.T) {
	t.Run("flat structures", func(t *testing.T) {
		m := &dtos.M{}
		defer dtos.Register[Ent, EntDTO](m, EntMapping{})()

		expEnt := Ent{V: rnd.Int()}
		expDTO := EntDTO{V: strconv.Itoa(expEnt.V)}

		dto, err := dtos.Map[EntDTO](m, expEnt)
		assert.NoError(t, err)
		assert.Equal(t, expDTO, dto)

		ent, err := dtos.Map[Ent](m, dto)
		assert.NoError(t, err)
		assert.Equal(t, expEnt, ent)
	})
	t.Run("nested structures", func(t *testing.T) {
		m := &dtos.M{}
		defer dtos.Register[Ent, EntDTO](m, EntMapping{})()
		defer dtos.Register[NestedEnt, NestedEntDTO](m, NestedEntMapping{})()

		expEnt := NestedEnt{ID: rnd.String(), Ent: Ent{V: rnd.Int()}}
		expDTO := NestedEntDTO{ID: expEnt.ID, Ent: EntDTO{V: strconv.Itoa(expEnt.Ent.V)}}

		dto, err := dtos.Map[NestedEntDTO](m, expEnt)
		assert.NoError(t, err)
		assert.Equal(t, expDTO, dto)

		ent, err := dtos.Map[NestedEnt](m, dto)
		assert.NoError(t, err)
		assert.Equal(t, expEnt, ent)
	})
}

func TestMap(t *testing.T) {
	t.Run("nil M given", func(t *testing.T) {
		_, err := dtos.Map[EntDTO, Ent](nil, Ent{V: rnd.Int()})
		assert.Error(t, err)
	})
	t.Run("happy", func(t *testing.T) {
		m := &dtos.M{}
		defer dtos.Register[Ent, EntDTO](m, EntMapping{})()

		expEnt := Ent{V: rnd.Int()}
		expDTO := EntDTO{V: strconv.Itoa(expEnt.V)}

		dto, err := dtos.Map[EntDTO](m, expEnt)
		assert.NoError(t, err)
		assert.Equal(t, expDTO, dto)

		ent, err := dtos.Map[Ent](m, dto)
		assert.NoError(t, err)
		assert.Equal(t, expEnt, ent)
	})
	t.Run("rainy", func(t *testing.T) {
		var (
			m   = &dtos.M{}
			ent = Ent{V: rnd.Int()}
			dto = EntDTO{V: strconv.Itoa(ent.V)}
		)

		_, err := dtos.Map[EntDTO](m, ent)
		assert.ErrorIs(t, err, dtos.ErrNoMapping)

		_, err = dtos.Map[Ent](m, dto)
		assert.ErrorIs(t, err, dtos.ErrNoMapping)

		defer dtos.Register[Ent, EntDTO](m, EntMapping{})()

		_, err = dtos.Map[Ent, Ent](m, ent)
		assert.ErrorIs(t, err, dtos.ErrNoMapping)
	})
	t.Run("ptr", func(t *testing.T) {
		m := &dtos.M{}
		defer dtos.Register[Ent, EntDTO](m, EntMapping{})()

		expEnt := Ent{V: rnd.Int()}
		expDTO := EntDTO{V: strconv.Itoa(expEnt.V)}

		dto, err := dtos.Map[*EntDTO](m, expEnt)
		assert.NoError(t, err)
		assert.NotNil(t, dto)
		assert.Equal(t, expDTO, *dto)
	})
}

func ExampleRegister() {
	// JSONMapping will contain mapping from entities to JSON DTO structures.
	var JSONMapping dtos.M
	// registering Ent <---> EntDTO mapping
	_ = dtos.Register[Ent, EntDTO](&JSONMapping, EntMapping{})
	// registering NestedEnt <---> NestedEntDTO mapping, which includes the mapping of the nested entities
	_ = dtos.Register[NestedEnt, NestedEntDTO](&JSONMapping, NestedEntMapping{})

	var v = NestedEnt{
		ID: "42",
		Ent: Ent{
			V: 42,
		},
	}

	dto, err := dtos.Map[NestedEntDTO](&JSONMapping, v)
	if err != nil { // handle err
		return
	}

	_ = dto // data mapped into a DTO and now ready for marshalling
	/*
		NestedEntDTO{
			ID: "42",
			Ent: EntDTO{
				V: "42",
			},
		}
	*/

	data, err := json.Marshal(dto)
	if err != nil { // handle error
		return
	}

	_ = data
	/*
		{
			"id": "42",
			"ent": {
				"v": "42"
			}
		}
	*/

}

type Ent struct {
	V int
}

type EntDTO struct {
	V string `json:"v"`
}

type EntMapping struct{}

func (EntMapping) ToDTO(_ *dtos.M, ent Ent) (EntDTO, error) {
	return EntDTO{V: strconv.Itoa(ent.V)}, nil
}

func (EntMapping) ToEnt(m *dtos.M, dto EntDTO) (Ent, error) {
	v, err := strconv.Atoi(dto.V)
	if err != nil {
		return Ent{}, err
	}
	return Ent{V: v}, nil
}

type NestedEnt struct {
	ID  string
	Ent Ent
}

type NestedEntDTO struct {
	ID  string `json:"id"`
	Ent EntDTO `json:"ent"`
}

type NestedEntMapping struct{}

func (NestedEntMapping) ToEnt(m *dtos.M, dto NestedEntDTO) (NestedEnt, error) {
	return NestedEnt{
		ID:  dto.ID,
		Ent: dtos.MustMap[Ent](m, dto.Ent),
	}, nil
}

func (NestedEntMapping) ToDTO(m *dtos.M, ent NestedEnt) (NestedEntDTO, error) {
	return NestedEntDTO{
		ID:  ent.ID,
		Ent: dtos.MustMap[EntDTO](m, ent.Ent),
	}, nil
}

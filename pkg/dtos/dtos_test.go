package dtos_test

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"

	"go.llib.dev/frameless/pkg/dtos"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/random"
)

var rnd = random.New(random.CryptoSeed{})

var _ dtos.MP = dtos.P[Ent, EntDTO]{}

func TestM(t *testing.T) {
	ctx := context.Background()
	t.Run("mapping T to itself T, passthrough mode without registration", func(t *testing.T) {
		expEnt := Ent{V: rnd.Int()}
		gotEnt, err := dtos.Map[Ent](ctx, expEnt)
		assert.NoError(t, err)
		assert.Equal(t, expEnt, gotEnt)
	})
	t.Run("flat structures", func(t *testing.T) {
		m := EntMapping{}
		defer dtos.Register[Ent, EntDTO](m.ToDTO, m.ToEnt)()

		expEnt := Ent{V: rnd.Int()}
		expDTO := EntDTO{V: strconv.Itoa(expEnt.V)}

		dto, err := dtos.Map[EntDTO](ctx, expEnt)
		assert.NoError(t, err)
		assert.Equal(t, expDTO, dto)

		ent, err := dtos.Map[Ent](ctx, dto)
		assert.NoError(t, err)
		assert.Equal(t, expEnt, ent)
	})
	t.Run("nested structures", func(t *testing.T) {
		em := EntMapping{}
		nem := NestedEntMapping{}
		defer dtos.Register[Ent, EntDTO](em.ToDTO, em.ToEnt)()
		defer dtos.Register[NestedEnt, NestedEntDTO](nem.ToDTO, nem.ToEnt)()

		expEnt := NestedEnt{ID: rnd.String(), Ent: Ent{V: rnd.Int()}}
		expDTO := NestedEntDTO{ID: expEnt.ID, Ent: EntDTO{V: strconv.Itoa(expEnt.Ent.V)}}

		dto, err := dtos.Map[NestedEntDTO](ctx, expEnt)
		assert.NoError(t, err)
		assert.Equal(t, expDTO, dto)

		ent, err := dtos.Map[NestedEnt](ctx, dto)
		assert.NoError(t, err)
		assert.Equal(t, expEnt, ent)
	})
	t.Run("override can be done with reregistering", func(t *testing.T) {
		em := EntMapping{}

		// initial setup
		defer dtos.Register[Ent, EntDTO](
			func(ctx context.Context, e Ent) (EntDTO, error) { return EntDTO{}, nil },
			func(ctx context.Context, ed EntDTO) (Ent, error) { return Ent{}, nil })()

		// override
		defer dtos.Register[Ent, EntDTO](em.ToDTO, em.ToEnt)()

		expEnt := Ent{V: rnd.Int()}
		expDTO := EntDTO{V: strconv.Itoa(expEnt.V)}

		dto, err := dtos.Map[EntDTO](ctx, expEnt)
		assert.NoError(t, err)
		assert.Equal(t, expDTO, dto)

		ent, err := dtos.Map[Ent](ctx, dto)
		assert.NoError(t, err)
		assert.Equal(t, expEnt, ent)
	})
}

func ExampleRegister_partialDTOMappingSupport() {
	// When we only need an Entity to EntityPartialDTO mapping.
	dtos.Register[Ent, EntPartialDTO](EntToEntPartialDTO, nil)()

	var (
		ctx = context.Background()
		v   = Ent{V: 42, N: 12}
	)

	partialDTO, err := dtos.Map[EntPartialDTO](ctx, v)
	_, _ = partialDTO, err
}

func ExampleMap() {
	var _ = dtos.Register[Ent, EntDTO]( // only once at the global level
		EntMapping{}.ToDTO,
		EntMapping{}.ToEnt,
	)
	var (
		ctx = context.Background()
		ent = Ent{V: 42, N: 12}
	)

	dto, err := dtos.Map[EntDTO](ctx, ent)
	if err != nil {
		panic(err)
	}

	gotEnt, err := dtos.Map[Ent](ctx, dto)
	if err != nil {
		panic(err)
	}

	_ = gotEnt == ent // true
}

func ExampleMap_sliceSyntaxSugar() {
	var _ = dtos.Register[Ent, EntDTO]( // only once at the global level
		EntMapping{}.ToDTO,
		EntMapping{}.ToEnt,
	)
	var (
		ctx  = context.Background()
		ents = []Ent{{V: 42, N: 12}}
	)

	// all individual value will be mapped
	res, err := dtos.Map[[]EntDTO](ctx, ents)
	if err != nil {
		panic(err)
	}
	_ = res // []EntDTO{V: "42", N: 12}
}

func TestMap(t *testing.T) {
	ctx := context.Background()
	t.Run("nil M given", func(t *testing.T) {
		_, err := dtos.Map[EntDTO, Ent](nil, Ent{V: rnd.Int()})
		assert.Error(t, err)
	})
	t.Run("happy", func(t *testing.T) {
		em := EntMapping{}
		defer dtos.Register[Ent, EntDTO](em.ToDTO, em.ToEnt)()
		expEnt := Ent{V: rnd.Int()}
		expDTO := EntDTO{V: strconv.Itoa(expEnt.V)}

		dto, err := dtos.Map[EntDTO](ctx, expEnt)
		assert.NoError(t, err)
		assert.Equal(t, expDTO, dto)

		ent, err := dtos.Map[Ent](ctx, dto)
		assert.NoError(t, err)
		assert.Equal(t, expEnt, ent)
	})
	t.Run("rainy", func(t *testing.T) {
		var (
			ent = Ent{V: rnd.Int()}
			dto = EntDTO{V: strconv.Itoa(ent.V)}
		)

		_, err := dtos.Map[EntDTO](ctx, ent)
		assert.ErrorIs(t, err, dtos.ErrNoMapping)

		_, err = dtos.Map[Ent](ctx, dto)
		assert.ErrorIs(t, err, dtos.ErrNoMapping)

		defer dtos.Register[Ent, EntDTO](EntMapping{}.ToDTO, EntMapping{}.ToEnt)()

		_, err = dtos.Map[EntDTO](ctx, ent)
		assert.NoError(t, err)
	})
	t.Run("value to pointer", func(t *testing.T) {
		defer dtos.Register[Ent, EntDTO](EntMapping{}.ToDTO, EntMapping{}.ToEnt)()

		expEnt := Ent{V: rnd.Int()}
		expDTO := EntDTO{V: strconv.Itoa(expEnt.V)}

		dto, err := dtos.Map[*EntDTO](ctx, expEnt)
		assert.NoError(t, err)
		assert.NotNil(t, dto)
		assert.Equal(t, expDTO, *dto)
	})
	t.Run("pointer to value", func(t *testing.T) {
		defer dtos.Register[Ent, EntDTO](EntMapping{}.ToDTO, EntMapping{}.ToEnt)()

		expEnt := Ent{V: rnd.Int()}
		expDTO := EntDTO{V: strconv.Itoa(expEnt.V)}

		dto, err := dtos.Map[EntDTO](ctx, &expEnt)
		assert.NoError(t, err)
		assert.NotNil(t, dto)
		assert.Equal(t, expDTO, dto)
	})
	t.Run("(To->From is missing) when we only need to map from entity to a dto and not the other way around, second argument to Register is optional", func(t *testing.T) {
		defer dtos.Register[Ent, EntPartialDTO](EntToEntPartialDTO, nil)()

		var (
			ctx = context.Background()
			v   = Ent{V: rnd.Int(), N: rnd.Int()}
		)

		partialDTO, err := dtos.Map[EntPartialDTO, Ent](ctx, v)
		assert.NoError(t, err)
		assert.Equal(t, partialDTO, EntPartialDTO{N: v.N})

		_, err = dtos.Map[Ent](ctx, partialDTO)
		assert.ErrorIs(t, err, dtos.ErrNoMapping)
	})
	t.Run("(From->To is missing) when we only need to map from entity to a dto and not the other way around, second argument to Register is optional", func(t *testing.T) {
		defer dtos.Register[EntPartialDTO, Ent](nil, EntToEntPartialDTO)()

		var (
			ctx = context.Background()
			v   = Ent{V: rnd.Int(), N: rnd.Int()}
		)

		partialDTO, err := dtos.Map[EntPartialDTO, Ent](ctx, v)
		assert.NoError(t, err)
		assert.Equal(t, partialDTO, EntPartialDTO{N: v.N})

		_, err = dtos.Map[Ent](ctx, partialDTO)
		assert.ErrorIs(t, err, dtos.ErrNoMapping)
	})
	t.Run("when no mapping is supplied to Register, it will panics about this", func(t *testing.T) {
		got := assert.Panic(t, func() { defer dtos.Register[Ent, EntDTO](nil, nil)() })
		assert.NotNil(t, got)
	})
	t.Run("[]T", func(t *testing.T) {
		defer dtos.Register[Ent, EntDTO](EntMapping{}.ToDTO, EntMapping{}.ToEnt)()

		ents := []Ent{
			{V: rnd.Int()},
			{V: rnd.Int()},
		}
		expDS := []EntDTO{
			{V: strconv.Itoa(ents[0].V)},
			{V: strconv.Itoa(ents[1].V)},
		}

		ds, err := dtos.Map[[]EntDTO](ctx, ents)
		assert.NoError(t, err)
		assert.NotNil(t, ds)
		assert.Equal(t, expDS, ds)
	})
	t.Run("no []T syntax sugar, when explicit slice type is registered", func(t *testing.T) {
		defer dtos.Register[Ent, EntDTO](EntMapping{}.ToDTO, EntMapping{}.ToEnt)()

		expectedMappedDTOs := []EntDTO{
			{V: strconv.Itoa(rnd.Int())},
		}
		defer dtos.Register[[]Ent, []EntDTO](func(ctx context.Context, ents []Ent) ([]EntDTO, error) {
			return expectedMappedDTOs, nil
		}, nil)()

		ents := []Ent{
			{V: rnd.Int()},
			{V: rnd.Int()},
		}

		ds, err := dtos.Map[[]EntDTO](ctx, ents)
		assert.NoError(t, err)
		assert.NotNil(t, ds)
		assert.Equal(t, expectedMappedDTOs, ds)
	})
}

func ExampleRegister() {
	// JSONMapping will contain mapping from entities to JSON DTO structures.
	// registering Ent <---> EntDTO mapping
	_ = dtos.Register[Ent, EntDTO](
		EntMapping{}.ToDTO,
		EntMapping{}.ToEnt,
	)
	// registering NestedEnt <---> NestedEntDTO mapping, which includes the mapping of the nested entities
	_ = dtos.Register[NestedEnt, NestedEntDTO](
		NestedEntMapping{}.ToDTO,
		NestedEntMapping{}.ToEnt,
	)

	var v = NestedEnt{
		ID: "42",
		Ent: Ent{
			V: 42,
		},
	}

	ctx := context.Background()
	dto, err := dtos.Map[NestedEntDTO](ctx, v)
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
	N int
}

type EntDTO struct {
	V string `json:"v"`
	N int    `json:"n"`
}

type EntPartialDTO struct {
	N int `json:"n"`
}

func EntToEntPartialDTO(ctx context.Context, ent Ent) (EntPartialDTO, error) {
	return EntPartialDTO{N: ent.N}, nil
}

type EntMapping struct{}

func (EntMapping) ToDTO(ctx context.Context, ent Ent) (EntDTO, error) {
	return EntDTO{V: strconv.Itoa(ent.V), N: ent.N}, nil
}

func (EntMapping) ToEnt(ctx context.Context, dto EntDTO) (Ent, error) {
	v, err := strconv.Atoi(dto.V)
	if err != nil {
		return Ent{}, err
	}
	return Ent{V: v, N: dto.N}, nil
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

func (NestedEntMapping) ToEnt(ctx context.Context, dto NestedEntDTO) (NestedEnt, error) {
	return NestedEnt{
		ID:  dto.ID,
		Ent: dtos.MustMap[Ent](ctx, dto.Ent),
	}, nil
}

func (NestedEntMapping) ToDTO(ctx context.Context, ent NestedEnt) (NestedEntDTO, error) {
	return NestedEntDTO{
		ID:  ent.ID,
		Ent: dtos.MustMap[EntDTO](ctx, ent.Ent),
	}, nil
}

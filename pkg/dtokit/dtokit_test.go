package dtokit_test

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"

	"go.llib.dev/frameless/pkg/dtokit"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/random"
)

var rnd = random.New(random.CryptoSeed{})

var _ dtokit.MP = dtokit.P[Ent, EntDTO]{}

func TestM(t *testing.T) {
	ctx := context.Background()
	t.Run("mapping T to itself T, passthrough mode without registration", func(t *testing.T) {
		expEnt := Ent{V: rnd.Int()}
		gotEnt, err := dtokit.Map[Ent](ctx, expEnt)
		assert.NoError(t, err)
		assert.Equal(t, expEnt, gotEnt)
	})
	t.Run("mapping T and its subtype, passthrough mode for subtype without registration", func(t *testing.T) {
		type SEnt Ent

		t.Run("ENT to DTO", func(t *testing.T) {
			expEnt := Ent{V: rnd.Int()}
			gotEnt, err := dtokit.Map[SEnt](ctx, expEnt)
			assert.NoError(t, err)
			assert.Equal(t, gotEnt.N, expEnt.N)
			assert.Equal(t, gotEnt.V, expEnt.V)
		})

		t.Run("*ENT to DTO", func(t *testing.T) {
			expEnt := Ent{V: rnd.Int()}
			gotEnt, err := dtokit.Map[SEnt](ctx, &expEnt)
			assert.NoError(t, err)
			assert.Equal(t, gotEnt.N, expEnt.N)
			assert.Equal(t, gotEnt.V, expEnt.V)
		})

		t.Run("ENT to *DTO", func(t *testing.T) {
			expEnt := Ent{V: rnd.Int()}
			gotEnt, err := dtokit.Map[*SEnt](ctx, expEnt)
			assert.NoError(t, err)
			assert.Equal(t, gotEnt.N, expEnt.N)
			assert.Equal(t, gotEnt.V, expEnt.V)
		})
	})
	t.Run("flat structures", func(t *testing.T) {
		m := EntMapping{}
		defer dtokit.Register[Ent, EntDTO](m.ToDTO, m.ToEnt)()

		expEnt := Ent{V: rnd.Int()}
		expDTO := EntDTO{V: strconv.Itoa(expEnt.V)}

		dto, err := dtokit.Map[EntDTO](ctx, expEnt)
		assert.NoError(t, err)
		assert.Equal(t, expDTO, dto)

		ent, err := dtokit.Map[Ent](ctx, dto)
		assert.NoError(t, err)
		assert.Equal(t, expEnt, ent)
	})
	t.Run("nested structures", func(t *testing.T) {
		em := EntMapping{}
		nem := NestedEntMapping{}
		defer dtokit.Register[Ent, EntDTO](em.ToDTO, em.ToEnt)()
		defer dtokit.Register[NestedEnt, NestedEntDTO](nem.ToDTO, nem.ToEnt)()

		expEnt := NestedEnt{ID: rnd.String(), Ent: Ent{V: rnd.Int()}}
		expDTO := NestedEntDTO{ID: expEnt.ID, Ent: EntDTO{V: strconv.Itoa(expEnt.Ent.V)}}

		dto, err := dtokit.Map[NestedEntDTO](ctx, expEnt)
		assert.NoError(t, err)
		assert.Equal(t, expDTO, dto)

		ent, err := dtokit.Map[NestedEnt](ctx, dto)
		assert.NoError(t, err)
		assert.Equal(t, expEnt, ent)
	})
	t.Run("override can be done with reregistering", func(t *testing.T) {
		em := EntMapping{}

		// initial setup
		defer dtokit.Register[Ent, EntDTO](
			func(ctx context.Context, e Ent) (EntDTO, error) { return EntDTO{}, nil },
			func(ctx context.Context, ed EntDTO) (Ent, error) { return Ent{}, nil })()

		// override
		defer dtokit.Register[Ent, EntDTO](em.ToDTO, em.ToEnt)()

		expEnt := Ent{V: rnd.Int()}
		expDTO := EntDTO{V: strconv.Itoa(expEnt.V)}

		dto, err := dtokit.Map[EntDTO](ctx, expEnt)
		assert.NoError(t, err)
		assert.Equal(t, expDTO, dto)

		ent, err := dtokit.Map[Ent](ctx, dto)
		assert.NoError(t, err)
		assert.Equal(t, expEnt, ent)
	})
}

func ExampleRegister_partialDTOMappingSupport() {
	// When we only need an Entity to EntityPartialDTO mapping.
	dtokit.Register[Ent, EntPartialDTO](EntToEntPartialDTO, nil)()

	var (
		ctx = context.Background()
		v   = Ent{V: 42, N: 12}
	)

	partialDTO, err := dtokit.Map[EntPartialDTO](ctx, v)
	_, _ = partialDTO, err
}

func ExampleMap() {
	var _ = dtokit.Register[Ent, EntDTO]( // only once at the global level
		EntMapping{}.ToDTO,
		EntMapping{}.ToEnt,
	)
	var (
		ctx = context.Background()
		ent = Ent{V: 42, N: 12}
	)

	dto, err := dtokit.Map[EntDTO](ctx, ent)
	if err != nil {
		panic(err)
	}

	gotEnt, err := dtokit.Map[Ent](ctx, dto)
	if err != nil {
		panic(err)
	}

	_ = gotEnt == ent // true
}

func ExampleMap_sliceSyntaxSugar() {
	var _ = dtokit.Register[Ent, EntDTO]( // only once at the global level
		EntMapping{}.ToDTO,
		EntMapping{}.ToEnt,
	)
	var (
		ctx  = context.Background()
		ents = []Ent{{V: 42, N: 12}}
	)

	// all individual value will be mapped
	res, err := dtokit.Map[[]EntDTO](ctx, ents)
	if err != nil {
		panic(err)
	}
	_ = res // []EntDTO{V: "42", N: 12}
}

func TestMap(t *testing.T) {
	ctx := context.Background()
	t.Run("nil M given", func(t *testing.T) {
		_, err := dtokit.Map[EntDTO, Ent](nil, Ent{V: rnd.Int()})
		assert.Error(t, err)
	})
	t.Run("happy", func(t *testing.T) {
		em := EntMapping{}
		defer dtokit.Register[Ent, EntDTO](em.ToDTO, em.ToEnt)()
		expEnt := Ent{V: rnd.Int()}
		expDTO := EntDTO{V: strconv.Itoa(expEnt.V)}

		dto, err := dtokit.Map[EntDTO](ctx, expEnt)
		assert.NoError(t, err)
		assert.Equal(t, expDTO, dto)

		ent, err := dtokit.Map[Ent](ctx, dto)
		assert.NoError(t, err)
		assert.Equal(t, expEnt, ent)
	})
	t.Run("rainy", func(t *testing.T) {
		var (
			ent = Ent{V: rnd.Int()}
			dto = EntDTO{V: strconv.Itoa(ent.V)}
		)

		_, err := dtokit.Map[EntDTO](ctx, ent)
		assert.ErrorIs(t, err, dtokit.ErrNoMapping)

		_, err = dtokit.Map[Ent](ctx, dto)
		assert.ErrorIs(t, err, dtokit.ErrNoMapping)

		defer dtokit.Register[Ent, EntDTO](EntMapping{}.ToDTO, EntMapping{}.ToEnt)()

		_, err = dtokit.Map[EntDTO](ctx, ent)
		assert.NoError(t, err)
	})
	t.Run("value to pointer", func(t *testing.T) {
		defer dtokit.Register[Ent, EntDTO](EntMapping{}.ToDTO, EntMapping{}.ToEnt)()

		expEnt := Ent{V: rnd.Int()}
		expDTO := EntDTO{V: strconv.Itoa(expEnt.V)}

		dto, err := dtokit.Map[*EntDTO](ctx, expEnt)
		assert.NoError(t, err)
		assert.NotNil(t, dto)
		assert.Equal(t, expDTO, *dto)
	})
	t.Run("pointer to value", func(t *testing.T) {
		defer dtokit.Register[Ent, EntDTO](EntMapping{}.ToDTO, EntMapping{}.ToEnt)()

		expEnt := Ent{V: rnd.Int()}
		expDTO := EntDTO{V: strconv.Itoa(expEnt.V)}

		dto, err := dtokit.Map[EntDTO](ctx, &expEnt)
		assert.NoError(t, err)
		assert.NotNil(t, dto)
		assert.Equal(t, expDTO, dto)
	})
	t.Run("(To->From is missing) when we only need to map from entity to a dto and not the other way around, second argument to Register is optional", func(t *testing.T) {
		defer dtokit.Register[Ent, EntPartialDTO](EntToEntPartialDTO, nil)()

		var (
			ctx = context.Background()
			v   = Ent{V: rnd.Int(), N: rnd.Int()}
		)

		partialDTO, err := dtokit.Map[EntPartialDTO, Ent](ctx, v)
		assert.NoError(t, err)
		assert.Equal(t, partialDTO, EntPartialDTO{N: v.N})

		_, err = dtokit.Map[Ent](ctx, partialDTO)
		assert.ErrorIs(t, err, dtokit.ErrNoMapping)
	})
	t.Run("(From->To is missing) when we only need to map from entity to a dto and not the other way around, second argument to Register is optional", func(t *testing.T) {
		defer dtokit.Register[EntPartialDTO, Ent](nil, EntToEntPartialDTO)()

		var (
			ctx = context.Background()
			v   = Ent{V: rnd.Int(), N: rnd.Int()}
		)

		partialDTO, err := dtokit.Map[EntPartialDTO, Ent](ctx, v)
		assert.NoError(t, err)
		assert.Equal(t, partialDTO, EntPartialDTO{N: v.N})

		_, err = dtokit.Map[Ent](ctx, partialDTO)
		assert.ErrorIs(t, err, dtokit.ErrNoMapping)
	})
	t.Run("when no mapping is supplied to Register, it will panics about this", func(t *testing.T) {
		got := assert.Panic(t, func() { defer dtokit.Register[Ent, EntDTO](nil, nil)() })
		assert.NotNil(t, got)
	})
	t.Run("[]T", func(t *testing.T) {
		defer dtokit.Register[Ent, EntDTO](EntMapping{}.ToDTO, EntMapping{}.ToEnt)()

		ents := []Ent{
			{V: rnd.Int()},
			{V: rnd.Int()},
		}
		expDS := []EntDTO{
			{V: strconv.Itoa(ents[0].V)},
			{V: strconv.Itoa(ents[1].V)},
		}

		ds, err := dtokit.Map[[]EntDTO](ctx, ents)
		assert.NoError(t, err)
		assert.NotNil(t, ds)
		assert.Equal(t, expDS, ds)
	})
	t.Run("no []T syntax sugar, when explicit slice type is registered", func(t *testing.T) {
		defer dtokit.Register[Ent, EntDTO](EntMapping{}.ToDTO, EntMapping{}.ToEnt)()

		expectedMappedDTOs := []EntDTO{
			{V: strconv.Itoa(rnd.Int())},
		}
		defer dtokit.Register[[]Ent, []EntDTO](func(ctx context.Context, ents []Ent) ([]EntDTO, error) {
			return expectedMappedDTOs, nil
		}, nil)()

		ents := []Ent{
			{V: rnd.Int()},
			{V: rnd.Int()},
		}

		ds, err := dtokit.Map[[]EntDTO](ctx, ents)
		assert.NoError(t, err)
		assert.NotNil(t, ds)
		assert.Equal(t, expectedMappedDTOs, ds)
	})
}

func ExampleRegister() {
	// JSONMapping will contain mapping from entities to JSON DTO structures.
	// registering Ent <---> EntDTO mapping
	_ = dtokit.Register[Ent, EntDTO](
		EntMapping{}.ToDTO,
		EntMapping{}.ToEnt,
	)
	// registering NestedEnt <---> NestedEntDTO mapping, which includes the mapping of the nested entities
	_ = dtokit.Register[NestedEnt, NestedEntDTO](
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
	dto, err := dtokit.Map[NestedEntDTO](ctx, v)
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
		Ent: dtokit.MustMap[Ent](ctx, dto.Ent),
	}, nil
}

func (NestedEntMapping) ToDTO(ctx context.Context, ent NestedEnt) (NestedEntDTO, error) {
	return NestedEntDTO{
		ID:  ent.ID,
		Ent: dtokit.MustMap[EntDTO](ctx, ent.Ent),
	}, nil
}

func ExampleMapping() {
	type Foo struct{ V int }

	type FooJSONDTO struct {
		V string `json:"v"`
	}

	_ = dtokit.Mapping[Foo, FooJSONDTO]{
		ToENT: func(ctx context.Context, dto FooJSONDTO) (Foo, error) {
			v, err := strconv.Atoi(dto.V)
			return Foo{V: v}, err
		},

		ToDTO: func(ctx context.Context, ent Foo) (FooJSONDTO, error) {
			return FooJSONDTO{V: strconv.Itoa(ent.V)}, nil
		},
	}
}

func TestMapping(t *testing.T) {
	type X struct{ V int }
	type XDTO struct{ V string }

	var ToEnt = func(ctx context.Context, dto XDTO) (X, error) {
		v, err := strconv.Atoi(dto.V)
		return X{V: v}, err
	}
	var ToDTO = func(ctx context.Context, ent X) (XDTO, error) {
		return XDTO{V: strconv.Itoa(ent.V)}, nil
	}

	t.Run("smoke", func(t *testing.T) {
		// create a Mapping that knows about Entity and DTO types,
		// then pass it to the dependent code that only knows about the Entity type,
		// but has to work with the DTO as part of the serialisation process
		var (
			m   dtokit.Mapper[X] = dtokit.Mapping[X, XDTO]{ToENT: ToEnt, ToDTO: ToDTO}
			ctx                  = context.Background()
			ent X                = X{V: rnd.Int()}
		)

		t.Log("map concrete entity type to a dto value (any)")
		dto, err := m.MapToiDTO(ctx, ent)
		assert.NoError(t, err)
		assert.NotNil(t, dto)
		assert.NotEmpty(t, dto)

		t.Log("serialize the dto value")
		data, err := json.Marshal(dto)
		assert.NoError(t, err)
		assert.NotEmpty(t, data)

		t.Log("prepare to unserialize the value")
		var ptrOfDTO any = m.NewiDTO()
		assert.NotNil(t, ptrOfDTO)
		rptr, ok := ptrOfDTO.(*XDTO)
		assert.True(t, ok)
		assert.NotNil(t, rptr)
		assert.Empty(t, *rptr)
		assert.NoError(t, json.Unmarshal(data, ptrOfDTO))
		assert.Equal[any](t, *rptr, dto)

		t.Log("map the untyped *DTO value back into an entity")
		gotEnt, err := m.MapFromiDTOPtr(ctx, ptrOfDTO)
		assert.NoError(t, err)
		assert.Equal(t, gotEnt, ent)
	})

	t.Run("default use dtokit register", func(t *testing.T) {
		// create a Mapping that knows about Entity and DTO types,
		// then pass it to the dependent code that only knows about the Entity type,
		// but has to work with the DTO as part of the serialisation process
		var (
			m   dtokit.Mapper[X] = dtokit.Mapping[X, XDTO]{}
			ctx                  = context.Background()
			ent X                = X{V: rnd.Int()}
		)

		t.Log("given the dtokit register has mapping between the entity and the dto")
		defer dtokit.Register(ToDTO, ToEnt)()

		t.Log("map concrete entity type to a dto value (any)")
		dto, err := m.MapToiDTO(ctx, ent)
		assert.NoError(t, err)
		assert.NotNil(t, dto)
		assert.NotEmpty(t, dto)

		t.Log("serialize the dto value")
		data, err := json.Marshal(dto)
		assert.NoError(t, err)
		assert.NotEmpty(t, data)

		t.Log("prepare to unserialize the value")
		var ptrOfDTO any = m.NewiDTO()
		assert.NotNil(t, ptrOfDTO)
		rptr, ok := ptrOfDTO.(*XDTO)
		assert.True(t, ok)
		assert.NotNil(t, rptr)
		assert.Empty(t, *rptr)
		assert.NoError(t, json.Unmarshal(data, ptrOfDTO))
		assert.Equal[any](t, *rptr, dto)

		t.Log("map the untyped *DTO value back into an entity")
		gotEnt, err := m.MapFromiDTOPtr(ctx, ptrOfDTO)
		assert.NoError(t, err)
		assert.Equal(t, gotEnt, ent)
	})

	t.Run("MapToENT+MapToDTO - default fallback using dtokit.Map", func(t *testing.T) {
		var (
			m     = dtokit.Mapping[X, XDTO]{}
			ctx   = context.Background()
			ent X = X{V: rnd.Int()}
		)

		t.Log("given the dtokit register has mapping between the entity and the dto")
		defer dtokit.Register(ToDTO, ToEnt)()

		t.Log("map concrete entity type to a dto value (any)")
		dto, err := m.MapToDTO(ctx, ent)
		assert.NoError(t, err)
		assert.NotNil(t, dto)
		assert.NotEmpty(t, dto)

		t.Log("serialize the dto value")
		data, err := json.Marshal(dto)
		assert.NoError(t, err)
		assert.NotEmpty(t, data)

		t.Log("prepare to unserialize the value")
		var gotDTO XDTO
		assert.NoError(t, json.Unmarshal(data, &gotDTO))
		assert.Equal(t, gotDTO, dto)

		t.Log("map the untyped *DTO value back into an entity")
		gotEnt, err := m.MapToENT(ctx, gotDTO)
		assert.NoError(t, err)
		assert.Equal(t, gotEnt, ent)
	})

	t.Run("MapToENT+MapToDTO - using the func fields", func(t *testing.T) {
		var (
			m = dtokit.Mapping[X, XDTO]{
				ToENT: ToEnt,
				ToDTO: ToDTO,
			}
			ctx   = context.Background()
			ent X = X{V: rnd.Int()}
		)

		t.Log("map concrete entity type to a dto value (any)")
		dto, err := m.MapToDTO(ctx, ent)
		assert.NoError(t, err)
		assert.NotNil(t, dto)
		assert.NotEmpty(t, dto)

		t.Log("serialize the dto value")
		data, err := json.Marshal(dto)
		assert.NoError(t, err)
		assert.NotEmpty(t, data)

		t.Log("prepare to unserialize the value")
		var gotDTO XDTO
		assert.NoError(t, json.Unmarshal(data, &gotDTO))
		assert.Equal(t, gotDTO, dto)

		t.Log("map the untyped *DTO value back into an entity")
		gotEnt, err := m.MapToENT(ctx, gotDTO)
		assert.NoError(t, err)
		assert.Equal(t, gotEnt, ent)
	})
}

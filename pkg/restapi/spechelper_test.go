package restapi_test

import (
	"context"
	"encoding/json"
	"go.llib.dev/frameless/pkg/dtos"
	"go.llib.dev/frameless/pkg/restapi"
	"go.llib.dev/testcase"
	"net/http/httptest"
	"strconv"
)

type (
	Foo struct {
		ID  FooID
		Foo int
	}
	FooID int
)

type FooDTO struct {
	ID  int `json:"id"`
	Foo int `json:"foo"`
}

var JSONMapping = dtos.M{}

var _ = dtos.Register[Foo, FooDTO](&JSONMapping, FooMapping{})

type FooMapping struct {
	restapi.IntID[FooID]
	restapi.IDInContext[FooMapping, FooID]
	restapi.SetIDByExtIDTag[Foo, FooID]
}

func (f FooMapping) ToEnt(m *dtos.M, dto FooDTO) (Foo, error) {
	return Foo{ID: FooID(dto.ID), Foo: dto.Foo}, nil
}

func (f FooMapping) ToDTO(m *dtos.M, ent Foo) (FooDTO, error) {
	return FooDTO{ID: int(ent.ID), Foo: ent.Foo}, nil
}

func (f FooMapping) MapEntity(ctx context.Context, dto FooDTO) (Foo, error) {
	return Foo{ID: FooID(dto.ID), Foo: dto.Foo}, nil
}

func (f FooMapping) MapDTO(ctx context.Context, entity Foo) (FooDTO, error) {
	return FooDTO{ID: int(entity.ID), Foo: entity.Foo}, nil
}

type Bar struct {
	ID  string
	Bar int
}

type BarDTO struct {
	ID  string `json:"id"`
	Bar int    `json:"bar"`
}

type BarMapping struct {
	restapi.StringID[string]
	restapi.SetIDByExtIDTag[Bar, string]
	restapi.IDInContext[BarMapping, string]
}

func (f BarMapping) MapEntity(ctx context.Context, dto BarDTO) (Bar, error) {
	return Bar{ID: dto.ID, Bar: dto.Bar}, nil
}

func (f BarMapping) MapDTO(ctx context.Context, entity Bar) (BarDTO, error) {
	return BarDTO{ID: entity.ID, Bar: entity.Bar}, nil
}

type BazID int

type Baz struct {
	ID  BazID
	Baz int
}

type BazDTO struct {
	ID  BazID `json:"id"`
	Baz int   `json:"baz"`
}

func MakeBazMapping() BazMapping {
	return BazMapping{
		IDConverter: restapi.IDConverter[int]{
			Format: func(id int) (string, error) {
				return strconv.Itoa(id), nil
			},
			Parse: strconv.Atoi,
		},
	}
}

type BazMapping struct {
	restapi.IDConverter[int]
	restapi.SetIDByExtIDTag[Baz, string]
	restapi.IDInContext[BazMapping, string]
}

func (f BazMapping) MapEntity(ctx context.Context, dto BazDTO) (Baz, error) {
	return Baz{ID: dto.ID, Baz: dto.Baz}, nil
}

func (f BazMapping) MapDTO(ctx context.Context, entity Baz) (BazDTO, error) {
	return BazDTO{ID: entity.ID, Baz: entity.Baz}, nil
}

func respondsWithJSON[DTO any](t *testcase.T, recorder *httptest.ResponseRecorder) DTO {
	var dto DTO
	t.Log("body:", recorder.Body.String())
	t.Must.NotEmpty(recorder.Body.Bytes())
	t.Must.NoError(json.Unmarshal(recorder.Body.Bytes(), &dto))
	return dto
}

package restapi_test

import (
	"context"
	"go.llib.dev/frameless/pkg/restapi"
	"strconv"
)

type (
	Foo struct {
		ID  FooID
		Foo int
	}
	FooID = int
)

type FooDTO struct {
	ID  int `json:"id"`
	Foo int `json:"foo"`
}

type FooMapping struct {
	restapi.IntID[int]
	restapi.IDInContext[FooMapping, int]
	restapi.SetIDByExtIDTag[Foo, int]
}

func (f FooMapping) MapEntity(ctx context.Context, dto FooDTO) (Foo, error) {
	return Foo{ID: dto.ID, Foo: dto.Foo}, nil
}

func (f FooMapping) MapDTO(ctx context.Context, entity Foo) (FooDTO, error) {
	return FooDTO{ID: entity.ID, Foo: entity.Foo}, nil
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

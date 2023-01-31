package restapi_test

import (
	"context"

	"github.com/adamluzsi/frameless/pkg/restapi/restmapping"
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
	restmapping.IntID
	restmapping.IDInContext[FooMapping, int]
	restmapping.SetIDByExtIDTag[Foo, int]
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
	restmapping.StringID
	restmapping.SetIDByExtIDTag[Bar, string]
	restmapping.IDInContext[BarMapping, string]
}

func (f BarMapping) MapEntity(ctx context.Context, dto BarDTO) (Bar, error) {
	return Bar{ID: dto.ID, Bar: dto.Bar}, nil
}

func (f BarMapping) MapDTO(ctx context.Context, entity Bar) (BarDTO, error) {
	return BarDTO{ID: entity.ID, Bar: entity.Bar}, nil
}

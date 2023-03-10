package testent

import (
	"context"
	"github.com/adamluzsi/frameless/ports/pubsub"
	"github.com/adamluzsi/testcase"
	"testing"
)

type (
	Foo struct {
		ID  FooID `ext:"ID"`
		Foo string
		Bar string
		Baz string
	}
	FooID string
)

func MakeFoo(tb testing.TB) Foo {
	te := tb.(*testcase.T).Random.Make(Foo{}).(Foo)
	te.ID = ""
	return te
}

type FooDTO struct {
	ID  string `ext:"ID" json:"id"`
	Foo string `json:"foo"`
	Bar string `json:"bar"`
	Baz string `json:"baz"`
}

type FooJSONMapping struct{}

func (n FooJSONMapping) ToDTO(ent Foo) (FooDTO, error) {
	return FooDTO{ID: string(ent.ID), Foo: ent.Foo, Bar: ent.Bar, Baz: ent.Baz}, nil
}

func (n FooJSONMapping) ToEnt(dto FooDTO) (Foo, error) {
	return Foo{ID: FooID(dto.ID), Foo: dto.Foo, Bar: dto.Bar, Baz: dto.Baz}, nil
}

func MakeContext(tb testing.TB) context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	tb.Cleanup(cancel)
	return ctx
}

type FooQueueID string

type FooQueue struct {
	ID FooQueueID `ext:"id"`
	pubsub.Publisher[Foo]
	pubsub.Subscriber[Foo]
}

func (fq FooQueue) SetPublisher(p pubsub.Publisher[Foo])   { fq.Publisher = p }
func (fq FooQueue) SetSubscriber(s pubsub.Subscriber[Foo]) { fq.Subscriber = s }

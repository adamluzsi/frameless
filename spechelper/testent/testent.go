package testent

import (
	"context"
	"go.llib.dev/frameless/pkg/dtos"
	"go.llib.dev/frameless/ports/pubsub"
	"go.llib.dev/testcase"
	"testing"
)

type Foo struct {
	ID  FooID `ext:"ID"`
	Foo string
	Bar string
	Baz string
}

func (foo Foo) LookupID() (FooID, bool) {
	return foo.ID, foo.ID != ""
}

type FooID string

func (id FooID) String() string { return string(id) }

func (f Foo) GetFoo() string {
	return f.Foo
}

func MakeFoo(tb testing.TB) Foo {
	te := testcase.ToT(&tb).Random.Make(Foo{}).(Foo)
	te.ID = ""
	return te
}

func MakeFooFunc(tb testing.TB) func() Foo {
	return func() Foo { return MakeFoo(tb) }
}

var _ = dtos.Register[Foo, FooDTO](func(ctx context.Context, foo Foo) (FooDTO, error) {
	return FooDTO{
		ID:   string(foo.ID),
		FooV: foo.Foo,
		BarV: foo.Bar,
		BazV: foo.Baz,
	}, nil
}, func(ctx context.Context, dto FooDTO) (Foo, error) {
	return Foo{
		ID:  FooID(dto.ID),
		Foo: dto.FooV,
		Bar: dto.BarV,
		Baz: dto.BazV,
	}, nil
})

type FooDTO struct {
	ID   string `ext:"ID" json:"id"`
	FooV string `json:"foov"`
	BarV string `json:"barv"`
	BazV string `json:"bazv"`
}

type FooJSONMapping struct{}

func (n FooJSONMapping) ToDTO(ent Foo) (FooDTO, error) {
	return FooDTO{ID: string(ent.ID), FooV: ent.Foo, BarV: ent.Bar, BazV: ent.Baz}, nil
}

func (n FooJSONMapping) ToEnt(dto FooDTO) (Foo, error) {
	return Foo{ID: FooID(dto.ID), Foo: dto.FooV, Bar: dto.BarV, Baz: dto.BazV}, nil
}

func MakeContextFunc(tb testing.TB) func() context.Context {
	return func() context.Context {
		ctx, cancel := context.WithCancel(context.Background())
		tb.Cleanup(cancel)
		return ctx
	}
}

type FooQueueID string

type FooQueue struct {
	ID FooQueueID `ext:"id"`
	pubsub.Publisher[Foo]
	pubsub.Subscriber[Foo]
}

func (fq FooQueue) SetPublisher(p pubsub.Publisher[Foo])   { fq.Publisher = p }
func (fq FooQueue) SetSubscriber(s pubsub.Subscriber[Foo]) { fq.Subscriber = s }

type Fooer interface {
	GetFoo() string
}

type Bar struct {
	ID BarID `ext:"id"`

	N int
	C string
}

type BarID string

func (id BarID) String() string { return string(id) }

type BarJSONDTO struct {
	ID string `json:"id"`
	N  int    `json:"number"`
	C  string `json:"char"`
}

var _ = dtos.Register[Bar, BarJSONDTO](func(ctx context.Context, bar Bar) (BarJSONDTO, error) {
	return BarJSONDTO{
		ID: string(bar.ID),
		N:  bar.N,
		C:  bar.C,
	}, nil
}, func(ctx context.Context, jsondto BarJSONDTO) (Bar, error) {
	return Bar{
		ID: BarID(jsondto.ID),
		N:  jsondto.N,
		C:  jsondto.C,
	}, nil
})

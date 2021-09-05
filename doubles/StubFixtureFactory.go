package doubles

import (
	"context"

	"github.com/adamluzsi/frameless"
)

type StubFixtureFactory struct {
	frameless.FixtureFactory
	StubContext func() context.Context
	StubCreate  func(T interface{}) interface{}
}

func (s StubFixtureFactory) Fixture(T interface{}, ctx context.Context) interface{} {
	if s.StubCreate != nil {
		return s.StubCreate(T)
	}
	return s.FixtureFactory.Fixture(T, nil)
}

//func (s StubFixtureFactory) Context() context.Context {
//	if s.StubContext != nil {
//		return s.StubContext()
//	}
//	return s.FixtureFactory.Context()
//}

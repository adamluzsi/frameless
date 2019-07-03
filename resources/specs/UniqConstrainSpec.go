package specs

import (
	"github.com/adamluzsi/frameless/reflects"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

type UniqConstrainSpec struct {
	// Struct that is the subject of this spec
	EntityType interface{}
	FixtureFactory

	// the combination of which the values must be uniq
	// The values for this are the struct Fields that together represent a uniq constrain
	// if you only want to make uniq one certain field across the resource,
	// then you only have to provide that only value in the slice
	UniqConstrain []string

	// the resource object that implements the specification
	Subject MinimumRequirements
}

func (spec UniqConstrainSpec) Test(t *testing.T) {
	require.Nil(t, spec.Subject.Truncate(spec.Context(spec.EntityType), spec.EntityType))

	e1 := spec.FixtureFactory.Create(spec.EntityType)
	e2 := spec.FixtureFactory.Create(spec.EntityType)
	e3 := spec.FixtureFactory.Create(spec.EntityType)

	v1 := reflect.ValueOf(e1)
	v2 := reflect.ValueOf(e2)
	v3 := reflect.ValueOf(e2)

	for _, field := range spec.UniqConstrain {
		val := v1.Elem().FieldByName(field)

		if val.Kind() == reflect.Invalid {
			t.Fatalf(`field %s is not found in %s`, field, reflects.FullyQualifiedName(spec.EntityType))
		}

		v2.Elem().FieldByName(field).Set(val)
		v3.Elem().FieldByName(field).Set(val)
	}

	require.Nil(t, spec.Subject.Save(spec.Context(spec.EntityType), e1))
	require.Error(t,
		spec.Subject.Save(spec.Context(spec.EntityType), e2),
		`expected that this value is cannot be saved since uniq constrain prevent it`,
	)

	t.Log(`after we delete the value that keeps the uniq constrain`)
	id, found := LookupID(e1)
	require.True(t, found)
	require.Nil(t, spec.Subject.DeleteByID(spec.Context(spec.EntityType), e1, id))

	t.Logf(`it should allow us to save similar object in the resource`)
	require.Nil(t, spec.Subject.Save(spec.Context(spec.EntityType), e3))

}

func TestUniqConstrain(t *testing.T, r MinimumRequirements, e interface{}, f FixtureFactory, uniqConstrain ...string) {
	t.Run(`UniqConstrainSpec`, func(t *testing.T) {
		require.NotEmpty(t, uniqConstrain)
		UniqConstrainSpec{EntityType: e, FixtureFactory: f, UniqConstrain: uniqConstrain, Subject: r}.Test(t)
	})
}

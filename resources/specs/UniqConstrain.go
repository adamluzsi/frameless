package specs

import (
	"reflect"
	"testing"

	"github.com/adamluzsi/frameless/reflects"
	"github.com/adamluzsi/frameless/resources"

	"github.com/stretchr/testify/require"
)

type uniqConstraint struct {
	// Struct that is the subject of this spec
	T interface{}
	FixtureFactory

	// the combination of which the values must be uniq
	// The values for this are the struct Fields that together represent a uniq constrain
	// if you only want to make uniq one certain field across the Resource,
	// then you only have to provide that only value in the slice
	UniqConstrain []string

	// the Resource object that implements the specification
	Subject minimumRequirements
}

func (spec uniqConstraint) Benchmark(b *testing.B) {
	b.Skip(msgNotMeasurable)
}

func (spec uniqConstraint) Test(t *testing.T) {
	DeleteAllEntity(t, spec.Subject, spec.Context(), spec.T)

	e1 := spec.FixtureFactory.Create(spec.T)
	e2 := spec.FixtureFactory.Create(spec.T)
	e3 := spec.FixtureFactory.Create(spec.T)

	v1 := reflect.ValueOf(e1)
	v2 := reflect.ValueOf(e2)
	v3 := reflect.ValueOf(e2)

	for _, field := range spec.UniqConstrain {
		val := v1.Elem().FieldByName(field)

		if val.Kind() == reflect.Invalid {
			t.Fatalf(`field %s is not found in %s`, field, reflects.FullyQualifiedName(spec.T))
		}

		v2.Elem().FieldByName(field).Set(val)
		v3.Elem().FieldByName(field).Set(val)
	}

	require.Nil(t, spec.Subject.Create(spec.Context(), e1))
	require.Error(t,
		spec.Subject.Create(spec.Context(), e2),
		`expected that this value is cannot be saved since uniq constrain prevent it`,
	)

	t.Log(`after we delete the value that keeps the uniq constrain`)
	id, found := resources.LookupID(e1)
	require.True(t, found)
	require.Nil(t, spec.Subject.DeleteByID(spec.Context(), e1, id))

	t.Logf(`it should allow us to save similar object in the resource`)
	require.Nil(t, spec.Subject.Create(spec.Context(), e3))

}

func TestUniqConstrain(t *testing.T, r minimumRequirements, e interface{}, f FixtureFactory, uniqConstrain ...string) {
	t.Run(`uniqConstraint`, func(t *testing.T) {
		require.NotEmpty(t, uniqConstrain)
		uniqConstraint{T: e, FixtureFactory: f, UniqConstrain: uniqConstrain, Subject: r}.Test(t)
	})
}

package frameless

import "context"

type FixtureFactory interface {
	// Fixture will create a fixture data for the provided T type.
	// The created fixture data expected to have random data in its fields.
	// It is expected that the created fixture will have no content for extID field.
	Fixture(T interface{}, ctx context.Context) (_T interface{})
}

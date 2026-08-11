package memory

import (
	"sync"

	"go.llib.dev/frameless/pkg/reflectkit"
)

// getNamespaceFor gives back the namespace string, or if empty, then set the namespace to the type value.
//
// once is the per-Repository sync.Once that guards the lazy initialisation of
// the namespace field, so concurrent readers (e.g. testcase.Race) can call
// getNamespaceFor without racing on the field write.
func getNamespaceFor[T any](typ string, namespace *string, once *sync.Once) string {
	once.Do(func() {
		if len(*namespace) == 0 {
			*namespace = reflectkit.FullyQualifiedName[T](*new(T))
		}
	})
	return typ + "/" + *namespace
}

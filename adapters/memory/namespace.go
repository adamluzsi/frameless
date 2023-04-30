package memory

import "github.com/adamluzsi/frameless/pkg/reflectkit"

// getNamespaceFor gives back the namespace string, or if empty, then set the namespace to the type value
func getNamespaceFor[T any](typ string, namespace *string) string {
	if len(*namespace) == 0 {
		*namespace = reflectkit.FullyQualifiedName(*new(T))
	}
	return typ + "/" + *namespace
}

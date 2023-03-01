package memory

import "github.com/adamluzsi/frameless/pkg/reflects"

// getNamespaceFor gives back the namespace string, or if empty, then set the namespace to the type value
func getNamespaceFor[T any](typ string, namespace *string) string {
	if len(*namespace) == 0 {
		*namespace = reflects.FullyQualifiedName(*new(T))
	}
	return typ + "/" + *namespace
}

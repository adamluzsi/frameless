package reflectkit

import (
	"fmt"
)

func FullyQualifiedName(e interface{}) string {
	t := BaseTypeOf(e)

	if t.PkgPath() == "" {
		return fmt.Sprintf("%s", t.Name())
	}

	return fmt.Sprintf("%q.%s", t.PkgPath(), t.Name())
}

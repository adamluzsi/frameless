package reflects

import (
	"fmt"
	"github.com/adamluzsi/frameless"
)

func FullyQualifiedName(e frameless.Entity) string {
	t := BaseTypeOf(e)

	if t.PkgPath() == "" {
		return fmt.Sprintf("%s", t.Name())
	}

	return fmt.Sprintf("%q.%s", t.PkgPath(), t.Name())
}

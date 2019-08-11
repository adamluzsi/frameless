package reflects

import (
	"fmt"
	"github.com/adamluzsi/frameless"
	"path/filepath"
)

func SymbolicName(e frameless.Entity) string {
	t := BaseTypeOf(e)

	if t.PkgPath() == "" {
		return fmt.Sprintf("%s", t.Name())
	}

	return fmt.Sprintf("%s.%s", filepath.Base(t.PkgPath()), t.Name())
}

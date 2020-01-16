package reflects

import (
	"fmt"
	"path/filepath"
)

func SymbolicName(e interface{}) string {
	t := BaseTypeOf(e)

	if t.PkgPath() == "" {
		return fmt.Sprintf("%s", t.Name())
	}

	return fmt.Sprintf("%s.%s", filepath.Base(t.PkgPath()), t.Name())
}

# package zeroutil

The zeroutil package helps with zero value related use-cases such as initialisation.

```go
package mypkg

import (
	"go.llib.dev/frameless/pkg/zerokit"
)

type MyType struct {
	V string
}

func (mt *MyType) getV() string {
	return zeroutil.Init(&mt.V, func() string {
		return "default-value"
	})
}

```
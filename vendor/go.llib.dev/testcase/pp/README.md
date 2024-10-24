<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->

- [Pretty Print (PP)](#pretty-print-pp)
	- [usage](#usage)
		- [PP / Format](#pp--format)
		- [Diff](#diff)
	- [printing into a file](#printing-into-a-file)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

# Pretty Print (PP)

the `pp` package provides you with a set of tools that pretty print any Go value.

## usage

### PP / Format

```go
package main

import (
	"bytes"
	"encoding/json"
	"go.llib.dev/testcase/pp"
)

type ExampleStruct struct {
	A string
	B int
}

func main() {
	var buf bytes.Buffer
	bs, _ := json.Marshal(ExampleStruct{
		A: "The Answer",
		B: 42,
	})
	buf.Write(bs)

	pp.PP(buf)
}
```

> output

```
bytes.Buffer{
        buf: []byte(`{"A":"The Answer","B":42}`),
        off: 0,
        lastRead: 0,
}
```

### Diff

```go
package main

import (
	"fmt"
	"go.llib.dev/testcase/pp"
)

type ExampleStruct struct {
	A string
	B int
}

func main(t *testing.T) {
	fmt.Println(pp.Diff(ExampleStruct{
		A: "The Answer",
		B: 42,
	}, ExampleStruct{
		A: "The Question",
		B: 42,
	}))
}
```

> output in GNU diff side-by-side style

```go
pp_test.ExampleStruct{     pp_test.ExampleStruct{
  A: "The Answer",      |    A: "The Question",
  B: 42,                     B: 42,
}   
```

## printing into a file

If STDOUT is supressed, you can also instruct PP to print into a file by setting the output file path in the `PP` environment variable.

```shell
export PP="out.txt"
```

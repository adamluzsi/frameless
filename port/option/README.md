# option port

The option port is an idiomatic way to introduce optional arguments and default values to your functions
using the variadic parameter syntax.


```go
func MyFunctionWithOptions(arg1 string, arg2 int, opts ...Option) {
    conf := option.Use[config](opts)
    // ...
}
```

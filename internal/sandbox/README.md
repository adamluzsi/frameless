# Sandbox

Sandbox lets you run a function in a safe, isolated environment and provides clear feedback if any panic or goexit occurs.

```sh
go test -run - -bench .
```

|                             | Nanoseconds per Operation |
| --------------------------- | ------------------------- |
| `sandbox.Run`               | 360.8 ns/op               |
| `go` + `recover` in `defer` | 336.0 ns/op               |

> **Environment**:  
>
> - OS: `darwin`  
> - Arch: `arm64`  
> - Test duration: 3.421s

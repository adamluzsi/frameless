# Package `units`

The units package in Go provides a set of constants for commonly used units, like the size units for digital data.

These constants can be used to represent byte sizes in a more readable and consistent manner,
similar to how the time package provides constants for durations like seconds and minutes.

```go
bs := make([]byte, units.Megabyte)
```

```go
r := io.LimitReader(buf, 128*units.Kibibyte)
```

For debugging purposes, you can even format a value which represent byte size with `FormatByteSize`.

```go
fmt.Println(units.FormatByteSize(n))
// 1.51KiB
```

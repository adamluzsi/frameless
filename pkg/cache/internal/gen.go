//go:generate mkdir -p memory
//go:generate rm -f ./memory/*.go
//go:generate cp ../../../adapter/memory/guard.go ./memory/guard.go
//go:generate cp ../../../adapter/memory/guard_test.go ./memory/guard_test.go
//go:generate go test ./memory
package internal

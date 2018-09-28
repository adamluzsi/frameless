package storages

import "github.com/adamluzsi/frameless"

type Core struct {
	queryImplementations map[frameless.Query] func(frameless.Query) frameless.Iterator
}

package migration

import "context"

type Migratable interface {
	Migrate(context.Context) error
}

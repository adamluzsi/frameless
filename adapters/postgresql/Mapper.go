package postgresql

import (
	"context"

	"github.com/adamluzsi/frameless/ports/iterators"
)

type Mapper[Entity, ID any] struct {
	// Table is the entity's table name
	Table string
	// ID is the entity's id column name
	ID string
	// Columns hold the entity's column names
	Columns []string

	NewIDFn  func(ctx context.Context) (ID, error)
	ToArgsFn func(ptr *Entity) ([]interface{}, error)
	MapFn    iterators.SQLRowMapperFunc[Entity]
}

func (m Mapper[Entity, ID]) TableRef() string {
	return m.Table
}

func (m Mapper[Entity, ID]) IDRef() string {
	return m.ID
}

func (m Mapper[Entity, ID]) ColumnRefs() []string {
	return m.Columns
}

func (m Mapper[Entity, ID]) NewID(ctx context.Context) (interface{}, error) {
	return m.NewIDFn(ctx)
}

func (m Mapper[Entity, ID]) ToArgs(ptr *Entity) ([]interface{}, error) {
	return m.ToArgsFn(ptr)
}

func (m Mapper[Entity, ID]) Map(s iterators.SQLRowScanner) (Entity, error) {
	return m.MapFn(s)
}

package postgresql

import (
	"context"

	"github.com/adamluzsi/frameless/pkg/iterators"
)

type Mapper[Ent, ID any] struct {
	// Table is the entity's table name
	Table string
	// ID is the entity's id column name
	ID string
	// Columns hold the entity's column names
	Columns []string

	NewIDFn  func(ctx context.Context) (ID, error)
	ToArgsFn func(ptr *Ent) ([]interface{}, error)
	MapFn    iterators.SQLRowMapperFunc[Ent]
}

func (m Mapper[Ent, ID]) TableRef() string {
	return m.Table
}

func (m Mapper[Ent, ID]) IDRef() string {
	return m.ID
}

func (m Mapper[Ent, ID]) ColumnRefs() []string {
	return m.Columns
}

func (m Mapper[Ent, ID]) NewID(ctx context.Context) (interface{}, error) {
	return m.NewIDFn(ctx)
}

func (m Mapper[Ent, ID]) ToArgs(ptr *Ent) ([]interface{}, error) {
	return m.ToArgsFn(ptr)
}

func (m Mapper[Ent, ID]) Map(s iterators.SQLRowScanner) (Ent, error) {
	return m.MapFn(s)
}

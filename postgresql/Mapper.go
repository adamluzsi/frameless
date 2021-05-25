package postgresql

import (
	"context"

	"github.com/adamluzsi/frameless/iterators"
)

type Mapper struct {
	// Table is the entity's table name
	Table string
	// ID is the entity's id column name
	ID string
	// Columns hold the entity's column names
	Columns []string

	NewIDFn  func(ctx context.Context) (interface{}, error)
	ToArgsFn func(ptr interface{}) ([]interface{}, error)
	MapFn    iterators.SQLRowMapperFunc
}

func (m Mapper) TableName() string {
	return m.Table
}

func (m Mapper) IDName() string {
	return m.ID
}

func (m Mapper) ColumnNames() []string {
	return m.Columns
}

func (m Mapper) NewID(ctx context.Context) (interface{}, error) {
	return m.NewIDFn(ctx)
}

func (m Mapper) ToArgs(ptr interface{}) ([]interface{}, error) {
	return m.ToArgsFn(ptr)
}

func (m Mapper) Map(s iterators.SQLRowScanner, ptr interface{}) error {
	return m.MapFn(s, ptr)
}

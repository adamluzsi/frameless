package flsql

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strings"
	"time"

	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/pkg/slicekit"
	"go.llib.dev/frameless/port/comproto"
	"go.llib.dev/frameless/port/crud/extid"
)

// Connection represent an open connection.
// Connection will respect the transaction state in the received context.Context.
type Connection interface {
	comproto.OnePhaseCommitProtocol
	Queryable
	io.Closer
}

type Queryable interface {
	ExecContext(ctx context.Context, query string, args ...any) (Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) Row
}

type Result interface {
	// RowsAffected returns the number of rows affected by an
	// update, insert, or delete. Not every database or database
	// driver may support this.
	RowsAffected() (int64, error)
}

type Rows interface {
	// Closer is the interface that wraps the basic Close method.
	io.Closer
	// Err returns any error that occurred while reading.
	Err() error
	// Next prepares the next row for reading. It returns true if there is another
	// row and false if no more rows are available. It automatically closes rows
	// when all rows are read.
	Next() bool
	// Scan reads the values from the current row into dest values positionally.
	// dest can include pointers to core types, values implementing the Scanner
	// interface, and nil. nil will skip the value entirely. It is an error to
	// call Scan without first calling Next() and checking that it returned true.
	Scan(dest ...any) error
}

type Row interface {
	// Scan works the same as Rows. with the following exceptions. If no
	// rows were found it returns errNoRows. If multiple rows are returned it
	// ignores all but the first.
	Scan(dest ...any) error
}

func MakeErrSQLRow(err error) sql.Row {
	var r sql.Row
	srrv := reflect.ValueOf(&r)
	reflectkit.SetValue(srrv.Elem().FieldByName("err"), reflect.ValueOf(err))
	return r
}

type ColumnName string

func JoinColumnName(cns []ColumnName, format string, sep string) string {
	return strings.Join(slicekit.Map(cns, func(n ColumnName) string { return fmt.Sprintf(format, n) }), sep)
}

type Scanner interface{ Scan(dest ...any) error }

type MapScan[ENT any] func(v *ENT, s Scanner) error

func (ms MapScan[ENT]) Map(scanner Scanner) (ENT, error) {
	var value ENT
	err := ms(&value, scanner)
	return value, err
}

// Mapping is a table mapping
type Mapping[ENT, ID any] struct {
	// TableName is the name of the table in the database.
	TableName string
	// ToQuery suppose to return back with the column names that needs to be selected from the table,
	// and the corresponding scan function that
	// ctx enables you to accept custom query instructions through the context if you require that.
	ToQuery func(ctx context.Context) ([]ColumnName, MapScan[ENT])
	// QueryID will convert an ID into query components—specifically,
	// column names and their corresponding values—that represent the ID in an SQL WHERE statement.
	// If ID is nil, then
	QueryID func(id ID) (QueryArgs, error)
	// ToArgs converts an entity pointer into a list of query arguments for CREATE or UPDATE operations.
	// It must handle empty or zero values and still return a valid column statement.
	ToArgs func(ENT) (QueryArgs, error)
	// CreatePrepare is an optional field that allows you to configure an entity prior to crud.Create call.
	// This is a good place to add support in your Repository implementation for custom ID injection or special timestamp value arrangement.
	//
	// To have this working, the user of Mapping needs to call Mapping.OnCreate method within in its crud.Create method implementation.
	CreatePrepare func(context.Context, *ENT) error
	// ID [optional] is a function that allows the ID lookup from an entity.
	// The returned ID value will be used to Lookup the ID value, or to set a new ID value.
	// Mapping will panic if ID func is provided, but returns a nil, as it is considered as implementation error.
	//
	// Example implementation:
	//
	// 		flsql.Mapping[Foo, FooID]{..., ID: func(v Foo) *FooID { return &v.ID }}
	//
	// default: extid.Lookup, extid.Set, which will use either the `ext:"id"` tag, or the `ENT.ID()` & `ENT.SetID()` methods.
	ID extid.Accessor[ENT, ID]
}

type QueryArgs map[ColumnName]any

func (m Mapping[ENT, ID]) OnCreate(ctx context.Context, ptr *ENT) error {
	// TODO: add support for CreatedAt, UpdatedAt field updates
	if m.CreatePrepare != nil {
		if err := m.CreatePrepare(ctx, ptr); err != nil {
			return err
		}
	}
	return nil
}

func SplitArgs(cargs map[ColumnName]any) ([]ColumnName, []any) {
	var (
		cols []ColumnName
		args []any
	)
	for col, arg := range cargs {
		cols = append(cols, col)
		args = append(args, arg)
	}
	return cols, args
}

// func (m Mapping[ENT, ID]) ErrSplitArgs(cargs map[ColumnName]any, err error) ([]ColumnName, []any, error) {
// 	if err != nil {
// 		return nil, nil, err
// 	}
// 	cols, args := m.SplitArgs(cargs)
// 	return cols, args, nil
// }

func QueryableSQL[SQLQ sqlQueryable](q SQLQ) Queryable {
	return QueryableAdapter[SQLQ]{
		V: q,
		ExecFunc: func(ctx context.Context, query string, args ...any) (Result, error) {
			r, err := q.ExecContext(ctx, query, args...)
			return r, err
		},
		QueryFunc: func(ctx context.Context, query string, args ...any) (Rows, error) {
			return q.QueryContext(ctx, query, args...)
		},
		QueryRowFunc: func(ctx context.Context, query string, args ...any) Row {
			return q.QueryRowContext(ctx, query, args...)
		},
	}
}

type sqlQueryable interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// DTO (Data Transfer Object) is an object used to transfer data between the database and your application.
// It acts as a bridge between the entity field types in your application and the table column types in your database,
// to making it easier to map data between them.
// This helps keep the data structure consistent when passing information between layers or systems.
type DTO interface {
	driver.Valuer
	sql.Scanner
}

func JSON[T any](pointer *T) *DTOJSON[T] {
	return &DTOJSON[T]{Pointer: pointer}
}

type DTOJSON[T any] struct {
	Pointer *T

	DisallowUnknownFields bool
}

func (m DTOJSON[T]) Value() (driver.Value, error) {
	if m.Pointer == nil {
		return nil, nil
	}
	return m.MarshalJSON()
}

func (m *DTOJSON[T]) Scan(value any) error {
	if value == nil {
		return nil
	}
	var data json.RawMessage
	switch value := value.(type) {
	case []byte:
		data = value
	case json.RawMessage:
		data = value
	case string:
		data = []byte(value)
	default:
		return fmt.Errorf("%T is not yet supported for %T", value, m)
	}
	return m.UnmarshalJSON(data)
}

func (m *DTOJSON[T]) MarshalJSON() ([]byte, error) {
	return json.Marshal(*m.Pointer)
}

func (m *DTOJSON[T]) UnmarshalJSON(data []byte) error {
	if m.DisallowUnknownFields {
		dec := json.NewDecoder(bytes.NewReader(data))
		dec.DisallowUnknownFields()
		return dec.Decode(&m.Pointer)
	}
	return json.Unmarshal(data, &m.Pointer)
}

func Timestamp(pointer *time.Time, layout string, tz *time.Location) DTO {
	return &dtoTimestamp{
		Pointer:  pointer,
		Layout:   layout,
		Location: tz,
	}
}

type dtoTimestamp struct {
	Pointer  *time.Time
	Layout   string
	Location *time.Location
}

func (m *dtoTimestamp) Scan(value any) error {
	if value == nil {
		return nil
	}
	switch value := value.(type) {
	case []byte:
		timestamp, err := m.parse(string(value))
		if err != nil {
			return err
		}
		*m.Pointer = timestamp
	case string:
		timestamp, err := m.parse(value)
		if err != nil {
			return err
		}
		*m.Pointer = timestamp
	case time.Time:
		if m.Location != nil {
			value = value.In(m.Location)
		}
		*m.Pointer = value
	default:
		return fmt.Errorf("scanning %T type as Timestamp is not yet supported", value)
	}
	return nil
}

func (m *dtoTimestamp) Value() (driver.Value, error) {
	if m.Pointer == nil {
		return nil, nil
	}
	t := *m.Pointer
	if m.Location != nil {
		t = t.In(m.Location)
	}
	if m.Layout == "" {
		return nil, fmt.Errorf("time formatting error: the format layout is empty")
	}
	formatted := t.Format(m.Layout)
	if formatted == m.Layout {
		val, err := m.parse(formatted)
		if err != nil {
			return nil, err
		}
		if !t.Equal(val) {
			return nil, fmt.Errorf("time formatting error: invalid layout: %s", m.Layout)
		}
	}
	return formatted, nil
}

func (m *dtoTimestamp) parse(raw string) (time.Time, error) {
	if m.Location != nil {
		return time.ParseInLocation(m.Layout, raw, m.Location)
	}
	return time.Parse(m.Layout, raw)
}

type MigrationStep[C Queryable] struct {
	Up      func(C, context.Context) error
	UpQuery string

	Down      func(C, context.Context) error
	DownQuery string
}

func (m MigrationStep[C]) MigrateUp(c C, ctx context.Context) error {
	if m.Up != nil {
		return m.Up(c, ctx)
	}
	if m.UpQuery != "" {
		_, err := c.ExecContext(ctx, m.UpQuery)
		return err
	}
	return nil
}

func (m MigrationStep[C]) MigrateDown(c C, ctx context.Context) error {
	if m.Down != nil {
		return m.Down(c, ctx)
	}
	if m.DownQuery != "" {
		_, err := c.ExecContext(ctx, m.DownQuery)
		return err
	}
	return nil
}

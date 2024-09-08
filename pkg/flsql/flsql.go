package flsql

import (
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
	"go.llib.dev/frameless/pkg/zerokit"
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

func JoinColumnName(cns []ColumnName, sep string, format string) string {
	return strings.Join(slicekit.Map(cns, func(n ColumnName) string { return fmt.Sprintf(format, n) }), sep)
}

type (
	ScanFunc func(dest ...any) error
	Scanner  interface{ Scan(dest ...any) error }
)

type MapScan[ENT any] func(v *ENT, scan ScanFunc) error

func (ms MapScan[ENT]) Map(scanner Scanner) (ENT, error) {
	var value ENT
	err := ms(&value, scanner.Scan)
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
	// ToID will convert an ID into query components—specifically,
	// column names and their corresponding values—that represent the ID in an SQL WHERE statement.
	// If ID is nil, then
	ToID func(id ID) (map[ColumnName]any, error)
	// ToArgs converts an entity pointer into a list of query arguments for CREATE or UPDATE operations.
	// It must handle empty or zero values and still return a valid column statement.
	ToArgs func(ENT) (map[ColumnName]any, error)
	// CreatePrepare is an optional field that allows you to configure an entity prior to crud.Create call.
	// This is a good place to add support in your Repository implementation for custom ID injection or special timestamp value arrangement.
	//
	// To have this working, the user of Mapping needs to call Mapping.OnCreate method within in its crud.Create method implementation.
	CreatePrepare func(context.Context, *ENT) error

	// GetID [optional] is a function that allows the ID lookup from an entity.
	//
	// default: extid.Lookup
	GetID func(ENT) ID
}

func (m Mapping[ENT, ID]) LookupID(ent ENT) (ID, bool) {
	if m.GetID != nil {
		var id ID = m.GetID(ent)
		return id, !zerokit.IsZero[ID](id)
	}
	return extid.Lookup[ID](ent)
}

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

func JSON[T any](pointer *T) DTO {
	return &dtoJSON[T]{Pointer: pointer}
}

type dtoJSON[T any] struct{ Pointer *T }

func (m dtoJSON[T]) Value() (driver.Value, error) {
	if m.Pointer == nil {
		return nil, nil
	}
	return json.Marshal(*m.Pointer)
}

func (m *dtoJSON[T]) Scan(value any) error {
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
	var raw string
	switch value := value.(type) {
	case []byte:
		raw = string(value)
	case string:
		raw = value
	default:
		return fmt.Errorf("scanning %T type as Timestamp is not yet supported", value)
	}
	timestamp, err := m.parse(raw)
	if err != nil {
		return err
	}
	*m.Pointer = timestamp
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

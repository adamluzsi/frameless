package postgresql

import (
	"context"
	"github.com/adamluzsi/frameless/iterators"
	"github.com/adamluzsi/frameless/resources"
)

type Mapping /* T */ interface {
	// TableName is the entity's postgresql table name.
	TableName() string
	// IDName is the entity's id column name, which can be used to access an individual record for update purpose.
	IDName() string
	// ColumnNames are the table's column names.
	// The order of the column names related to Row mapping and query argument passing.
	ColumnNames() []string
	// NewID creates a stateless entity id that can be used by CREATE operation.
	// Serial and similar id solutions not supported without serialize transactions.
	NewID(context.Context) (interface{}, error)
	// ToArgs convert an entity ptr to a list of query argument that can be used for CREATE or UPDATE purpose.
	ToArgs(ptr interface{}) ([]interface{}, error)
	iterators.SQLRowMapper
}

type Pool interface {
	// GetDSN returns the sql data source name.
	// This data source name should be usable with sql.Open
	GetDSN() string
	// GetClient returns the current context's sql client.
	// This can be a simple *sql.DB or if we within a transaction, then an *sql.Tx
	GetClient(ctx context.Context) (client SQLClient, free func(), err error)
	resources.OnePhaseCommitProtocol
}

package postgresql_test

import (
	"context"
	"github.com/adamluzsi/frameless/postgresql"
)

func ExampleConnectionManager() {
	connectionManager := postgresql.NewConnectionManager(`dsn`)
	defer connectionManager.Close()

	ctx := context.Background()

	c, err := connectionManager.GetConnection(ctx)
	if err != nil {
		panic(err)
	}

	_, err = c.ExecContext(ctx, `SELECT VERSION()`)
	if err != nil {
		panic(err)
	}
}

func ExampleConnectionManager_BeginTx() {
	connectionManager := postgresql.NewConnectionManager(`dsn`)
	defer connectionManager.Close()

	ctx := context.Background()

	tx, err := connectionManager.BeginTx(ctx)
	if err != nil {
		panic(err)
	}

	c, err := connectionManager.GetConnection(tx)
	if err != nil {
		panic(err)
	}

	_, err = c.ExecContext(tx, `SELECT VERSION()`)
	if err != nil {
		panic(err)
	}

	if err := connectionManager.CommitTx(tx); err != nil {
		panic(err)
	}
}

func ExampleConnectionManager_CommitTx() {
	connectionManager := postgresql.NewConnectionManager(`dsn`)
	defer connectionManager.Close()

	ctx := context.Background()

	tx, err := connectionManager.BeginTx(ctx)
	if err != nil {
		panic(err)
	}

	c, err := connectionManager.GetConnection(tx)
	if err != nil {
		panic(err)
	}

	_, err = c.ExecContext(tx, `SELECT VERSION()`)
	if err != nil {
		panic(err)
	}

	if err := connectionManager.CommitTx(tx); err != nil {
		panic(err)
	}
}

func ExampleConnectionManager_RollbackTx() {
	connectionManager := postgresql.NewConnectionManager(`dsn`)
	defer connectionManager.Close()

	ctx := context.Background()

	tx, err := connectionManager.BeginTx(ctx)
	if err != nil {
		panic(err)
	}

	c, err := connectionManager.GetConnection(tx)
	if err != nil {
		panic(err)
	}

	_, err = c.ExecContext(tx, `DROP TABLE xy`)
	if err != nil {
		panic(err)
	}

	if err := connectionManager.RollbackTx(tx); err != nil {
		panic(err)
	}
}

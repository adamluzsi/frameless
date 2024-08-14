package postgresql_test

import (
	"context"

	"go.llib.dev/frameless/adapter/postgresql"
)

func ExampleConnect() {
	c, err := postgresql.Connect(`dsn`)
	if err != nil {
		panic(err)
	}
	defer c.Close()

	_, err = c.ExecContext(context.Background(), `SELECT VERSION()`)
	if err != nil {
		panic(err)
	}
}

func ExampleConnection_BeginTx() {
	c, err := postgresql.Connect(`dsn`)
	if err != nil {
		panic(err)
	}
	defer c.Close()

	ctx := context.Background()

	tx, err := c.BeginTx(ctx)
	if err != nil {
		panic(err)
	}

	_, err = c.ExecContext(tx, `SELECT VERSION()`)
	if err != nil {
		panic(err)
	}

	if err := c.CommitTx(tx); err != nil {
		panic(err)
	}
}

func ExampleConnection_CommitTx() {
	c, err := postgresql.Connect(`dsn`)
	if err != nil {
		panic(err)
	}
	defer c.Close()

	ctx := context.Background()

	tx, err := c.BeginTx(ctx)
	if err != nil {
		panic(err)
	}

	_, err = c.ExecContext(tx, `SELECT VERSION()`)
	if err != nil {
		panic(err)
	}

	if err := c.CommitTx(tx); err != nil {
		panic(err)
	}
}

func ExampleConnection_RollbackTx() {
	c, err := postgresql.Connect(`dsn`)
	if err != nil {
		panic(err)
	}

	defer c.Close()

	ctx := context.Background()

	tx, err := c.BeginTx(ctx)
	if err != nil {
		panic(err)
	}

	_, err = c.ExecContext(tx, `DROP TABLE xy`)
	if err != nil {
		panic(err)
	}

	if err := c.RollbackTx(tx); err != nil {
		panic(err)
	}
}

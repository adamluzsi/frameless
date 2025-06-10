package mariadb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"go.llib.dev/frameless/pkg/zerokit"
)

func lookupSessionVariable[T any](conn Connection, ctx context.Context, key string) (T, bool, error) {
	var (
		name string
		val  T
	)
	row := conn.QueryRowContext(ctx, fmt.Sprintf("SHOW SESSION VARIABLES LIKE '%s'", key))
	err := row.Scan(&name, &val)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return *new(T), false, nil
		} else {
			return *new(T), false, err
		}
	}

	return val, true, nil
}

func withSessionVariable[T any](conn Connection, ctx context.Context, key string, val T) (func() error, error) {
	// Retrieve the current lock_wait_timeout value
	var (
		name string
		og   T
		had  bool = true
	)

	og, had, err := lookupSessionVariable[T](conn, ctx, key)
	if err != nil {
		return nil, err
	}

	row := conn.QueryRowContext(ctx, fmt.Sprintf("SHOW SESSION VARIABLES LIKE '%s'", key))
	if err := row.Scan(&name, &og); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			had = false
		} else {
			return nil, err
		}
	}

	// Set lock wait timeout to 1 second (adjust as needed)
	_, err = conn.ExecContext(ctx, fmt.Sprintf("SET SESSION %s = ?", key), val)
	if err != nil {
		return nil, err
	}

	return func() error {
		if had && !zerokit.IsZero(og) {
			_, err := conn.ExecContext(ctx, fmt.Sprintf("SET SESSION %s = ?", key), og)
			return err
		} else {
			_, err := conn.ExecContext(ctx, fmt.Sprintf("RESET SESSION %s", key))
			return err
		}
	}, nil
}

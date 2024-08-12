package main

import (
	"context"

	"go.llib.dev/frameless/internal/codemigrate"
)

var migrationSteps = []codemigrate.MigrationStep{
	codemigrate.MigrationStep{
		Up: func(ctx context.Context, r *codemigrate.Resource) error {
			r.Grep("frameless/ports/")

		},
	},
}

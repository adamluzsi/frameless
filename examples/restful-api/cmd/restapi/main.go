package main

import (
	"context"
	"restapi/adapter/httpapi"
	"restapi/domain/mydomain"

	"go.llib.dev/frameless/adapter/memory"
	"go.llib.dev/frameless/pkg/logger"
	"go.llib.dev/frameless/pkg/logging"
	"go.llib.dev/frameless/pkg/tasker"
)

func main() {
	ctx := logging.ContextWith(context.Background(), logger.Field("app", "restful-api-example"))
	err := Main(ctx)
	if err != nil {
		logger.Fatal(ctx, "error in main", logging.ErrField(err))
	}
}

func Main(ctx context.Context) error {
	c := httpapi.Config{
		UserRepository: &memory.Repository[mydomain.User, mydomain.UserID]{},
		NoteRepository: &memory.Repository[mydomain.Note, mydomain.NoteID]{},
	}

	srv, err := httpapi.NewServer(c)
	if err != nil {
		return err
	}

	return tasker.Main(ctx, tasker.HTTPServerTask(srv))
}

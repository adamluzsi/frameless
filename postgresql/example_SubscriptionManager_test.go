package postgresql_test

import (
	"context"
	"os"

	"github.com/adamluzsi/frameless/postgresql"
)

func ExampleSubscriptionManager() {
	type ExampleEntity struct {
		ID string `ext:"id"`
	}

	connectionManager := postgresql.NewConnectionManager(os.Getenv(`DATABASE_URL`))
	mapping := postgresql.Mapper{ /* real mapping data here */ }

	subscriptionManager, err := postgresql.NewSubscriptionManager(ExampleEntity{}, context.Background(), connectionManager, mapping)
	if err != nil {
		panic(err)
	}
	defer subscriptionManager.Close()
}

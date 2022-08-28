package postgresql_test

import (
	"os"

	"github.com/adamluzsi/frameless/adapters/postgresql"
)

func ExampleSubscriptionManager() {
	type ExampleEntity struct {
		ID string `ext:"id"`
	}

	connectionManager := postgresql.NewConnectionManager(os.Getenv(`DATABASE_URL`))
	mapping := postgresql.Mapper[ExampleEntity, string]{ /* real mapping data here */ }

	subscriptionManager := postgresql.NewListenNotifySubscriptionManager[ExampleEntity, string](mapping, os.Getenv("DATABASE_URL"), connectionManager)
	defer subscriptionManager.Close()
}

package postgresql_test

import (
	"os"

	"github.com/adamluzsi/frameless/postgresql"
)

func ExampleSubscriptionManager() {
	type ExampleEntity struct {
		ID string `ext:"id"`
	}

	connectionManager := postgresql.NewConnectionManager(os.Getenv(`DATABASE_URL`))
	mapping := postgresql.Mapper{ /* real mapping data here */ }

	subscriptionManager, err := postgresql.NewListenNotifySubscriptionManager(ExampleEntity{}, mapping, os.Getenv("DATABASE_URL"), connectionManager)
	if err != nil {
		panic(err)
	}
	defer subscriptionManager.Close()
}

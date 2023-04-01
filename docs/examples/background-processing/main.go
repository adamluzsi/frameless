package main

import (
	"context"
	"fmt"
	"github.com/adamluzsi/frameless/adapters/postgresql"
	"github.com/adamluzsi/frameless/pkg/logger"
	"github.com/adamluzsi/frameless/pkg/tasker"
	"github.com/adamluzsi/frameless/pkg/tasker/schedule"
	"github.com/adamluzsi/frameless/ports/comproto"
	"github.com/adamluzsi/frameless/ports/iterators"
	"github.com/adamluzsi/frameless/ports/pubsub"
	"net/http"
	"os"
)

func main() {
	ctx := context.Background()

	cm, err := postgresql.NewConnectionManagerWithDSN(os.Getenv("DATABASE_URL"))
	if err != nil {
		logger.Error(ctx, err.Error())
	}

	q := postgresql.Queue[MyDomainEventEntity, MyDomainEventEntityPGQueueJSONDTO]{
		Name:              "my_domain_event",
		ConnectionManager: cm,
		Mapping:           MappingForMyDomainEventEntity{},
	}

	myHTTPRequestHandler := MyHTTPRequestHandler{
		Publisher: q,
	}

	server := http.Server{
		Addr:    "localhost:8080",
		Handler: myHTTPRequestHandler,
	}

	// create my web app with graceful shutdown
	webAppTask := tasker.WithShutdown(tasker.IgnoreError(server.ListenAndServe, http.ErrServerClosed), server.Shutdown)

	// create my consumer task with error recovery + graceful shutdown
	myDomainEventConsumer := tasker.WithRepeat(schedule.Interval(0),
		// on error will recover HandleEvents when something goes wrong other than context cancellation
		tasker.OnError(MyDomainEventConsumer{Subscriber: q}.HandleEvents,
			func(ctx context.Context, err error) error {
				logger.Error(ctx, err.Error())
				return nil // ignore errors, let's recover!
			}))

	if err := tasker.Main(ctx,
		webAppTask,
		myDomainEventConsumer, // if you need more than one consumer per web node, you can pass it multiple times
	); err != nil {
		logger.Fatal(ctx, err.Error())
	}
}

// package mydomain

type MyDomainEventEntity struct {
	Foo string
	Bar int
	Baz bool
}

type MyDomainEventConsumer struct {
	pubsub.Subscriber[MyDomainEventEntity]
}

func (c MyDomainEventConsumer) HandleEvents(ctx context.Context) error {
	return iterators.ForEach(c.Subscriber.Subscribe(ctx), func(msg pubsub.Message[MyDomainEventEntity]) (rErr error) {
		defer comproto.FinishTx(&rErr, msg.ACK, msg.NACK)

		logger.Info(ctx, fmt.Sprint(msg.Data()))

		return nil // all good, I'm ok yay \o/
	})
}

// package myhttpapi

type MyHTTPRequestHandler struct {
	Publisher pubsub.Publisher[MyDomainEventEntity]
}

func (h MyHTTPRequestHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	err := h.Publisher.Publish(r.Context(), MyDomainEventEntity{
		Foo: r.URL.Query().Get("foo"),
		Bar: 42,
		Baz: !false, // it's funny because it's true
	})

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

// package mypostgresqladapter

type MyDomainEventEntityPGQueueJSONDTO struct {
	Foo string `json:"foo"`
	Bar int    `json:"bar"`
	Baz bool   `json:"baz"`
}

type MappingForMyDomainEventEntity struct{}

func (MappingForMyDomainEventEntity) ToDTO(ent MyDomainEventEntity) (MyDomainEventEntityPGQueueJSONDTO, error) {
	return MyDomainEventEntityPGQueueJSONDTO(ent), nil
}

func (MappingForMyDomainEventEntity) ToEnt(dto MyDomainEventEntityPGQueueJSONDTO) (MyDomainEventEntity, error) {
	return MyDomainEventEntity(dto), nil
}

# package frameless/ports

## Introduction

Welcome to Frameless, a powerful tool for streamlining your software development process and maintaining code
complexity.
Our primary goal is to provide easy-to-follow conventions that enhance your project's overall design
while remaining scalable, flexible, and maintainable.

Frameless achieves this by introducing hexagonal architecture-based ports and their corresponding contract,
also known as interface testing suites.
By integrating these concepts, Frameless ensures a clean separation of concerns,
enabling you to focus on writing high-quality code that adheres to best practices and delivers exceptional results.

By adhering to the conventions outlined by the Frameless project,
your software will boast a high degree of architectural flexibility,
empowering you to experiment with, replace, or remove adapter implementations as needed.
This adaptability ensures that even if you decide to remove Frameless from your project,
you can do so with minimal effort, and your software will continue to function seamlessly.
Embrace the freedom and resilience that Frameless brings to your development process,
and watch your projects thrive in a dynamic and ever-changing landscape.

### convention through frameless ports

Let's explore a concrete example to better understand the benefits of using Frameless ports in your project.

Consider a system where users can submit comments,
and a review process determines if the content is safe for public viewing.
By utilizing the CRUD port, you can effortlessly implement the repository pattern
and focus on defining the data in your domain. Furthermore,
your unit tests can employ a fake testing double that replicates the expected behavior of the final solution,
irrespective of the technology you ultimately choose.

```go
package mydomain

import (
	"context"
	"fmt"
	"go.llib.dev/frameless/ports/crud"
)

type (
	Comment struct {
		ID      CommentID
		Title   string
		Content string

		ReviewState string `enum:"accept;pending;reject;"`
	}
	CommentID string
)

type CommentAuditor struct {
	NoteRepository CommentRepository
}

func (nal CommentAuditor) Review(ctx context.Context, id CommentID) error {
	comment, found, err := nal.NoteRepository.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("note with id of %v is not found, review is not possible", id)
	}

	_ = comment.Title   // strict review logic for the title
	_ = comment.Content // strict review logic for the content
	comment.ReviewState = "accept"

	return nal.NoteRepository.Update(ctx, &comment)
}

type CommentRepository interface {
	crud.Creator[Comment]               // C
	crud.ByIDFinder[Comment, CommentID] // R
	crud.Updater[Comment]               // U
	crud.ByIDDeleter[CommentID]         // D
}
```

The CRUD interfaces showcased in the example are not extraordinary.
The function signatures they contain can be easily copied and pasted into your project,
and the conventions remain just as valid.

```go
package mydomain

type CommentRepository interface {
	Create(ctx context.Context, ptr *Comment) error
	FindByID(ctx context.Context, id CommentID) (ent Comment, found bool, err error)
	Update(ctx context.Context, ptr *Comment) error
	DeleteByID(ctx context.Context, id CommentID) error
}
```

At first glance, the signatures present in the frameless ports may appear simplistic.
However, rest assured that their simplicity results from countless iterations
and experimentation to control complexity and enhance ease of use.

### decreasing learning curve in the project through frameless ports/contracts

While an interface primarily aids in adhering to the Liskov substitution principle,
it does not necessarily invert dependency at the behavioural level.
Thus each Frameless port has its corresponding contract.
When a concrete implementation is devised for your domain code,
these contracts facilitate dependency inversion at the behavioural level,
allowing your domain layer to own and specify the behaviour.

Consider the following scenario:
you have a repository that retrieves your data, and missing data might result in an error for the sql.DB.QueryRow.
Rather than allowing this error to leak into your domain logic level,
the behavior should be inverted and communicated as "not found" using idiomatic Go code.

For instance, let's say the SQL package returns an ErrNoRows error when a query yields no results.
Without proper handling, this error can leak into your domain code as follows:

```go
package myadapter

func (r *commentRepository) FindByID(ctx context.Context, id CommentID) (Comment, error) {
	var comment Comment
	err := r.db.QueryRowContext(ctx, "SELECT * FROM comments WHERE id = $1", id).Scan(&comment.ID, &comment.Title, &comment.Content, &comment.ReviewState)
	if err != nil {
		return Comment{}, err
	}
	return comment, nil
}
```

To prevent this leakage, the combination of port and contract will point out the leakage with failing tests
thus encouraging you to encapsulate the implementation details in your adapter layer:

```go
package myadapter

func (r *commentRepository) FindByID(ctx context.Context, id CommentID) (Comment, bool, error) {
	var comment Comment
	err := r.db.QueryRowContext(ctx, "SELECT * FROM comments WHERE id = $1", id).Scan(&comment.ID, &comment.Title, &comment.Content, &comment.ReviewState)
	if errors.Is(err, sql.ErrNoRows) {
		// Inverted behaviour: ErrNoRows is translated to "not found",
		// which is not an error an actual error in the adapter layer, but instead,
		// it is up for interpretation in the domain layer
		return Comment{}, false, nil
	}
	if err != nil {
		return Comment{}, false, err
	}
	return comment, true, nil
}
```

Think of contracts as pre-written interface testing suites.
They effectively function like classicist Test-Driven Development tooling but on steroids.
By importing these pre-written contracts into your adapter's tests,
you can bypass the initial testing setup and dive straight into solution mode.
It also guarantees that the outcome will be easy to use by anyone who has experience
with something that implemented the same port, thus,
removing the learning curve with your components in the project for fresh starters.

#### practicing Consumer-Driven Contracts using pre-written frameless ports

First, let's do a quick recap on Consumer-Driven Contracts.
Consumer-Driven Contracts (CDC) is a testing approach that helps ensure different components
or services within a system can effectively communicate with each other.
In a CDC, consumers define the expectations they have from a provider,
which can then be used to create automated tests to verify that the provider meets these expectations.

##### How it support cross team communication

In this simplified example, let's consider two teams working on different services within a microservices architecture:
Both Team A and B is responsible for their services.
The Service developed by team A is one that provides their API (provider)
and Team B's service interact with that service (consumer).

- Team B, as the consumer, defines their expectations from the Team A's service
  and creates a contract specifying the API endpoints, request format, and expected response format.
    - If this API is something like a restful resource, Team B can import the framless crud contracts
      to define their expectations towards Team A's API.
    - if this API is more like a message bus based approach, Team B can import the frameless pubsub contracts
      to define their expectations towards Team A's API.
- Both Team A and B then integrates Team B's contract into their CI/CD pipeline
    - This cause each change from Team A to be verified by Team B's contract
- Suppose Team A makes a change to their service that inadvertently violates the contract
  (e.g., altering a field name in the API response).
  When Team A push the changes, their CI/CD pipeline runs the contract tests, which will now fail due to the violation.
- Then pipeline notifies Team A about the test failure,
  allowing them to identify and fix the issue before it reaches the staging environment or impacts Team B's service.
    - then Team A can decide whether to reach out to Team B to discuss the reason for breaking the API contract
      or they can apply further changes to honour the contract.
    - if Team A is not able/willing to run Team B's contract in their CI/CD pipeline,
      Team B can still regularly run the integration test,
      and reach out to Team A each time Team A broke the staging/integration environment.
- Once Team A resolves the issue, they push the updated code, and the pipeline runs the tests again.
  If the tests pass, the changes can be deployed to the staging environment and eventually to production.

By using frameless's tooling for composing Consumer-Driven contracts,
you can focus on establishing automated testing based communication with these shared integration tests as contract.

The CI/CD pipelines ensure that both teams adhere to the agreed-upon contract,
which helps detect and fix potential integration issues early in the development process,
preventing disruptions in the staging environment and fostering a more reliable system overall.

##### example

You as member of Team B, wish to use Team A's restful API.
You define a gateway interface, and within that you specify that you need access to foo resource resource.

- /foos/:foo_id

> ./yourdomain/teamaservice.go

```go
package yourdomain

type Foo struct {
	ID    string `ext:"id"`
	Value string
}

type FooRepository interface {
	crud.Creator[Foo]
	crud.ByIDFinder[Foo, string]
	crud.Updater[Foo]
	crud.ByIDDeleter[string]
}
```

Then under your `yourdomain` package, you can define a contract which will be supplied by an implementation:

> ./yourdomain/yourdomaincontracts/teamaservice.go

```go
package yourdomaincontracts

import (
	"context"
	"go.llib.dev/frameless/ports/crud/crudcontracts"
)

type FooRepository struct {
	MakeSubject func(tb testing.TB) yourdomain.FooRepository
	MakeContext func(tb testing.TB) context.Context
	MakeFoo     func(tb testing.TB) yourdomain.Foo
}

func (c FooRepository) Test(t *testing.T) {
	crudcontracts.Creator[yourdomain.Foo, string]{
		MakeSubject: ...
		MakeEntity: c.MakeFoo,
		MakeContext: c.MakeContext,
	}.Test(t)
	// other contracts imported depending what behaviour is required
}
```

Then importing this contract into your team A's service client adapter package will have all the domain logic
expectations

```go
package teamaserviceclient_test

func TestMyClient(t *testing.T) {
	yourdomaincontracts.FooRepository{
		// fill out the dependencies about how to create a teamaserviceclient.Client  
	}.Test(t)
}
```

Congrats, you just made a big pile of fine detailed integration tests against this client,
you are ready to switch into solution mode.

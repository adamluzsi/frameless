# Frameless [![Build Status](https://travis-ci.org/adamluzsi/frameless.svg?branch=master)](https://travis-ci.org/adamluzsi/frameless)

[GoDoc](https://pkg.go.dev/github.com/adamluzsi/frameless)

## Introduction 

Welcome to Frameless, a powerful tool for streamlining your software development process and maintaining code complexity.
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



Let's explore a concrete example to better understand the benefits of using Frameless in your project. 
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
	"github.com/adamluzsi/frameless/ports/crud"
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
func (r *commentRepository) FindByID(ctx context.Context, id CommentID) (Comment, bool, error) {
    var comment Comment
    err := r.db.QueryRowContext(ctx, "SELECT * FROM comments WHERE id = $1", id).Scan(&comment.ID, &comment.Title, &comment.Content, &comment.ReviewState)
    if err == sql.ErrNoRows {
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

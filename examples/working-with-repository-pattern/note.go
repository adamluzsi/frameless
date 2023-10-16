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

type CommentRepositoryWithoutEmbedding interface {
	Create(ctx context.Context, ptr *Comment) error
	FindByID(ctx context.Context, id CommentID) (ent Comment, found bool, err error)
	Update(ctx context.Context, ptr *Comment) error
	DeleteByID(ctx context.Context, id CommentID) error
}

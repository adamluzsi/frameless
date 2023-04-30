package restapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/adamluzsi/frameless/pkg/errorkit"
	"github.com/adamluzsi/frameless/pkg/restapi/rfc7807"
)

var ErrEntityNotFound = errorkit.UserError{
	ID:      "entity-not-found",
	Message: "The requested entity is not found in this resource.",
}

var ErrPathNotFound = errorkit.UserError{
	ID:      "path-not-found",
	Message: "The requested path is not found.",
}

var ErrEntityAlreadyExist = errorkit.UserError{
	ID:      "entity-already-exists",
	Message: "The entity could not be created as it already exists.",
}

var ErrMethodNotAllowed = errorkit.UserError{
	ID:      "restapi-method-not-allowed",
	Message: "The requested RESTful method is not supported.",
}

var ErrMalformedID = errorkit.UserError{
	ID:      "malformed-id-in-path",
	Message: "The received entity id in the path is malformed.",
}

var ErrInvalidRequestBody = errorkit.UserError{
	ID:      "invalid-create-request-body",
	Message: "The request body is invalid.",
}

var ErrInternalServerError = errorkit.UserError{
	ID:      "internal-server-error",
	Message: "An unexpected internal server error occurred.",
}

var ErrRequestEntityTooLarge = errorkit.UserError{
	ID:      "request-entity-too-large",
	Message: "The request body was larger than the size limit allowed for the server.",
}

var defaultErrorHandler = rfc7807.Handler{
	Mapping: ErrorMapping,
}

func ErrorMapping(ctx context.Context, err error, dto *rfc7807.DTO) {
	switch {
	case errors.Is(err, ErrInternalServerError):
		dto.Status = http.StatusInternalServerError
	case errors.Is(err, ErrMethodNotAllowed):
		dto.Status = http.StatusMethodNotAllowed
	case errors.Is(err, ErrEntityAlreadyExist):
		dto.Status = http.StatusConflict
	case errors.Is(err, ErrRequestEntityTooLarge):
		dto.Status = http.StatusRequestEntityTooLarge
	case errors.Is(err, ErrEntityNotFound),
		errors.Is(err, ErrPathNotFound):
		dto.Status = http.StatusNotFound
	case errors.Is(err, ErrMalformedID),
		errors.Is(err, ErrInvalidRequestBody):
		dto.Status = http.StatusBadRequest
	}
}

package httpkit

import (
	"context"
	"errors"
	"net/http"

	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/httpkit/rfc7807"
)

var ErrEntityNotFound = errorkit.UserError{
	Code:    "entity-not-found",
	Message: "The requested entity is not found in this resource.",
}

var ErrPathNotFound = errorkit.UserError{
	Code:    "path-not-found",
	Message: "The requested path is not found.",
}

var ErrEntityAlreadyExist = errorkit.UserError{
	Code:    "entity-already-exists",
	Message: "The entity could not be created as it already exists.",
}

var ErrForbidden = errorkit.UserError{
	Code:    "forbidden",
	Message: "Operation permanently forbidden. Repeating the request will yield the same result.",
}

var ErrMethodNotAllowed = errorkit.UserError{
	Code:    "rest-method-not-allowed",
	Message: "The requested RESTful method is not supported.",
}

var ErrMalformedID = errorkit.UserError{
	Code:    "malformed-id-in-path",
	Message: "The received entity id in the path is malformed.",
}

var ErrInvalidRequestBody = errorkit.UserError{
	Code:    "invalid-request-body",
	Message: "The request body is invalid.",
}

var ErrInternalServerError = errorkit.UserError{
	Code:    "internal-server-error",
	Message: "An unexpected internal server error occurred.",
}

var ErrRequestEntityTooLarge = errorkit.UserError{
	Code:    "request-entity-too-large",
	Message: "The request body was larger than the size limit allowed for the server.",
}

var ErrResponseEntityTooLarge = errorkit.UserError{
	Code:    "response-entity-too-large",
	Message: "The response body was larger than the size limit allowed for the client.",
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
	case errors.Is(err, ErrForbidden):
		dto.Type.ID = ErrForbidden.Code.String()
		dto.Status = http.StatusForbidden
		dto.Detail = ErrForbidden.Message.String()
	case errors.Is(err, ErrMalformedID),
		errors.Is(err, ErrInvalidRequestBody):
		dto.Status = http.StatusBadRequest
	}
}

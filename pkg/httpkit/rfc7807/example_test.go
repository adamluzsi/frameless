package rfc7807_test

import (
	"context"
	"errors"
	"net/http"

	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/httpkit/rfc7807"
)

type CompanyErrorStructure struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

const ErrSomeRandom errorkit.Error = "random error value"

func ErrorMapping(ctx context.Context, err error, dto *rfc7807.DTO) {
	switch {
	case errors.Is(err, ErrSomeRandom):
		dto.Type.ID = "some-random-err"
		dto.Detail = "this is a random error type"
	}
	dto.Extensions = CompanyErrorStructure{
		Code:    dto.Type.ID,
		Message: dto.Detail,
	}
}

func ExampleHandler_HandleError() {
	h := rfc7807.Handler{
		Mapping: ErrorMapping,
		BaseURL: "/errors",
	}

	_ = func(w http.ResponseWriter, r *http.Request) {
		h.HandleError(w, r, ErrSomeRandom)
	}
}

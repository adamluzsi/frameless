package rfc7807_test

import (
	"context"
	"errors"
	"github.com/adamluzsi/frameless/pkg/errorutil"
	"github.com/adamluzsi/frameless/pkg/restapi/rfc7807"
	"net/http"
)

type CompanyErrorStructure struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

const ErrSomeRandom errorutil.Error = "random error value"

func ErrorMapping(ctx context.Context, err error, dto *rfc7807.DTO[CompanyErrorStructure]) {
	switch {
	case errors.Is(err, ErrSomeRandom):
		dto.Type.ID = "some-random-err"
		dto.Detail = "this is a random error type"
	}
	dto.Extensions.Code = dto.Type.ID
	dto.Extensions.Message = dto.Detail
}

func ExampleHandler_HandleError() {
	h := rfc7807.Handler[CompanyErrorStructure]{
		Mapping: ErrorMapping,
		BaseURL: "/errors",
	}

	_ = func(w http.ResponseWriter, r *http.Request) {
		h.HandleError(w, r, ErrSomeRandom)
	}

}

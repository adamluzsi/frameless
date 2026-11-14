package httpkit_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"go.llib.dev/frameless/pkg/httpkit"
	"go.llib.dev/frameless/pkg/httpkit/rfc7807"
)

func TestErrorMapping(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantTypeID string
		wantDetail string
	}{
		{
			name:       "internal server error",
			err:        httpkit.ErrInternalServerError,
			wantStatus: http.StatusInternalServerError,
		},
		{
			name:       "method not allowed",
			err:        httpkit.ErrMethodNotAllowed,
			wantStatus: http.StatusMethodNotAllowed,
		},
		{
			name:       "entity already exists",
			err:        httpkit.ErrEntityAlreadyExist,
			wantStatus: http.StatusConflict,
		},
		{
			name:       "request entity too large",
			err:        httpkit.ErrRequestEntityTooLarge,
			wantStatus: http.StatusRequestEntityTooLarge,
		},
		{
			name:       "response unsupported media type",
			err:        httpkit.ErrResponseUnsupportedMediaType,
			wantStatus: http.StatusUnsupportedMediaType,
		},
		{
			name:       "entity not found",
			err:        httpkit.ErrEntityNotFound,
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "path not found",
			err:        httpkit.ErrPathNotFound,
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "forbidden",
			err:        httpkit.ErrForbidden,
			wantStatus: http.StatusForbidden,
			wantTypeID: string(httpkit.ErrForbidden.Code),
			wantDetail: string(httpkit.ErrForbidden.Message),
		},
		{
			name:       "malformed id",
			err:        httpkit.ErrMalformedID,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid request body",
			err:        httpkit.ErrInvalidRequestBody,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "wrapped internal server error",
			err:        httpkit.ErrInternalServerError.Wrap(errors.New("db down")),
			wantStatus: http.StatusInternalServerError,
		},
		{
			name:       "unmapped error leaves status unchanged",
			err:        errors.New("something unexpected"),
			wantStatus: http.StatusBadRequest,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dto := rfc7807.DTO{Status: http.StatusBadRequest}
			httpkit.ErrorMapping(context.Background(), tt.err, &dto)
			if dto.Status != tt.wantStatus {
				t.Errorf("Status = %d, want %d", dto.Status, tt.wantStatus)
			}
			if tt.wantTypeID != "" && dto.Type.ID != tt.wantTypeID {
				t.Errorf("Type.ID = %q, want %q", dto.Type.ID, tt.wantTypeID)
			}
			if tt.wantDetail != "" && dto.Detail != tt.wantDetail {
				t.Errorf("Detail = %q, want %q", dto.Detail, tt.wantDetail)
			}
		})
	}
}

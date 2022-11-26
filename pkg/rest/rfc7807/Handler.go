package rfc7807

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/adamluzsi/frameless/pkg/errorutil"
	"github.com/adamluzsi/frameless/pkg/internal/constant"
	"net/http"
	"strings"
)

type Handler[Extensions any] struct {
	// Mapping supplies the mapping logic that map the error value to a DTO[Extensions].
	Mapping HandlerMappingFunc[Extensions]
	// BaseURL is the URI path prefix the error types should have.
	// If none given, default is "/".
	BaseURL string
}

type HandlerMappingFunc[Extensions any] func(ctx context.Context, err error, dto *DTO[Extensions])

func (h Handler[Extensions]) HandleError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(r.Context().Err(), context.Canceled) && errors.Is(err, context.Canceled) {
		return
	}
	var (
		ctx        = r.Context()
		ID         string
		Title      string
		Detail     []string
		StatusCode int
	)
	if errCtx, ok := errorutil.LookupContext(err); ok {
		ctx = errCtx
	}
	if usrErr, ok := errorutil.LookupUserError(err); ok {
		ID = string(usrErr.ID)
		Title = h.toTitleCase(usrErr.ID)
		StatusCode = http.StatusBadRequest
		Detail = append(Detail, string(usrErr.Message))
	} else {
		ID = "internal-server-error"
		Title = http.StatusText(http.StatusInternalServerError)
		StatusCode = http.StatusInternalServerError
	}
	if detail, ok := errorutil.LookupDetail(err); ok {
		Detail = append(Detail, detail)
	}
	dto := DTO[Extensions]{
		Type: Type{
			ID:      ID,
			BaseURL: h.BaseURL,
		},
		Title:  Title,
		Status: StatusCode,
		Detail: strings.Join(Detail, "\n"),
	}
	if h.Mapping != nil {
		h.Mapping(ctx, err, &dto)
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(dto.Status)
	_ = json.NewEncoder(w).Encode(dto)
}

func (h Handler[Extensions]) toTitleCase(id constant.String) string {
	title := string(id)
	title = strings.ReplaceAll(title, "-", " ")
	title = strings.ReplaceAll(title, "_", " ")
	title = strings.ToTitle(title)
	return title
}

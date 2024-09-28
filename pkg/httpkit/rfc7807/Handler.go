package rfc7807

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"go.llib.dev/frameless/internal/constant"
	"go.llib.dev/frameless/pkg/errorkit"
)

type Handler struct {
	// Mapping supplies the mapping logic that map the error value to a DTO[Extensions].
	Mapping HandlerMappingFunc
	// BaseURL is the URI path prefix the error types should have.
	// If none given, default is "/".
	BaseURL string
}

type HandlerMappingFunc func(ctx context.Context, err error, dto *DTO)

func (h Handler) HandleError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(r.Context().Err(), context.Canceled) && errors.Is(err, context.Canceled) {
		return
	}
	var (
		ctx = r.Context()
	)
	dto := h.ToDTO(ctx, err)
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(dto.Status)
	bytes, err := json.Marshal(dto)
	if err != nil {
		fmt.Println("WARN", "rfc7807.Handler", "json.Marshal", err.Error())
		return
	}
	_, _ = w.Write(bytes)
}

func (h Handler) toTitleCase(id constant.String) string {
	title := string(id)
	title = strings.ReplaceAll(title, "-", " ")
	title = strings.ReplaceAll(title, "_", " ")
	title = strings.ToLower(title)
	if chars := []rune(title); 0 < len(chars) {
		fl := strings.ToUpper(string(chars[0:1]))
		title = fl + string(chars[1:])
	}
	return title
}

func (h Handler) ToDTO(ctx context.Context, err error) DTO {
	var (
		ID         string
		Title      string
		Detail     []string
		StatusCode int
	)
	if errCtx, ok := errorkit.LookupContext(err); ok {
		ctx = errCtx
	}
	if usrErr, ok := errorkit.LookupUserError(err); ok {
		ID = string(usrErr.ID)
		Title = h.toTitleCase(usrErr.ID)
		StatusCode = http.StatusBadRequest
		Detail = append(Detail, string(usrErr.Message))
	} else {
		ID = "internal-server-error"
		Title = http.StatusText(http.StatusInternalServerError)
		StatusCode = http.StatusInternalServerError
	}
	if detail, ok := errorkit.LookupDetail(err); ok {
		Detail = append(Detail, detail)
	}
	dto := DTO{
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
	return dto
}

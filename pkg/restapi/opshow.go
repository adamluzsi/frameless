package restapi

import (
	"encoding/json"
	"net/http"

	"go.llib.dev/frameless/ports/crud"
)

type ShowOperation[Entity, ID, DTO any] struct {
	BeforeHook BeforeHook
}

func (h Handler[Entity, ID, DTO]) show(w http.ResponseWriter, r *http.Request, id ID) {
	if !h.useBeforeHook(h.Operations.Show.BeforeHook, w, r) {
		return
	}

	finder, ok := h.Resource.(crud.ByIDFinder[Entity, ID])
	if !ok {
		h.errMethodNotAllowed(w, r)
		return
	}
	entity, found, err := finder.FindByID(r.Context(), id)
	if err != nil {
		h.getErrorHandler().HandleError(w, r, err)
		return
	}
	if !found {
		h.errEntityNotFound(w, r)
		return
	}

	dto, err := h.Mapping.MapDTO(r.Context(), entity)
	if err != nil {
		h.errInternalServerError(w, r, err)
		return
	}

	h.writeJSON(w, r, http.StatusOK, dto)
}

func (h Handler[Entity, ID, DTO]) writeJSON(w http.ResponseWriter, r *http.Request, code int, dto DTO) {
	bytes, err := json.Marshal(dto)
	if err != nil {
		h.errInternalServerError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_, _ = w.Write(bytes)
	_, _ = w.Write([]byte("\n"))
}

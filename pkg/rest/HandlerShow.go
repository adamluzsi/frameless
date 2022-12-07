package rest

import (
	"encoding/json"
	"github.com/adamluzsi/frameless/ports/crud"
	"net/http"
)

func (h Handler[Entity, ID, DTO]) show(w http.ResponseWriter, r *http.Request, id ID) {
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

package rest

import (
	"errors"
	"github.com/adamluzsi/frameless/ports/crud"
	"net/http"
)

func (h Handler[Entity, ID, DTO]) delete(w http.ResponseWriter, r *http.Request, id ID) {
	deleter, ok := h.Resource.(crud.ByIDDeleter[ID])
	if !ok {
		h.errMethodNotAllowed(w, r)
		return
	}

	ctx := r.Context()

	_, found, err := h.Resource.FindByID(ctx, id)
	if err != nil {
		h.getErrorHandler().HandleError(w, r, err)
		return
	}
	if !found {
		h.getErrorHandler().HandleError(w, r, ErrEntityNotFound)
		return
	}

	if err := deleter.DeleteByID(ctx, id); err != nil {
		if errors.Is(err, crud.ErrNotFound) {
			err = ErrEntityNotFound.With().Wrap(err)
		}
		h.getErrorHandler().HandleError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

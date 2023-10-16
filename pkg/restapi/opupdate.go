package restapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"go.llib.dev/frameless/ports/crud"
)

type UpdateOperation[Entity, ID, DTO any] struct {
	BeforeHook BeforeHook
}

func (h Handler[Entity, ID, DTO]) update(w http.ResponseWriter, r *http.Request, id ID) {
	if !h.useBeforeHook(h.Operations.Update.BeforeHook, w, r) {
		return
	}

	updater, ok := h.Resource.(crud.Updater[Entity])
	if !ok {
		h.errMethodNotAllowed(w, r)
		return
	}
	defer r.Body.Close()

	bodyReadLimit := h.getBodyReadLimit()
	bytes, err := io.ReadAll(io.LimitReader(r.Body, bodyReadLimit))
	if err != nil {
		defaultErrorHandler.HandleError(w, r,
			ErrRequestEntityTooLarge.With().
				Detailf("Body Limit: %d", bodyReadLimit))
		return
	}

	var dto DTO
	if err := json.Unmarshal(bytes, &dto); err != nil {
		h.getErrorHandler().HandleError(w, r,
			ErrInvalidRequestBody.With().
				Detail(err.Error()))
		return
	}

	ctx := r.Context()

	entity, err := h.Mapping.MapEntity(ctx, dto)
	if err != nil {
		h.getErrorHandler().HandleError(w, r, err)
		return
	}

	h.Mapping.SetID(&entity, id)

	_, found, err := h.Resource.FindByID(ctx, id)
	if err != nil {
		h.getErrorHandler().HandleError(w, r, err)
		return
	}
	if !found {
		h.getErrorHandler().HandleError(w, r, ErrEntityNotFound)
		return
	}

	if err := updater.Update(ctx, &entity); err != nil {
		if errors.Is(err, crud.ErrAlreadyExists) {
			err = ErrEntityAlreadyExist.With().Wrap(err)
		}
		h.getErrorHandler().HandleError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

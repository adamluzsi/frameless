package restapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/adamluzsi/frameless/ports/crud"
)

type CreateOperation[Entity, ID, DTO any] struct {
	BeforeHook BeforeHook
}

func (h Handler[Entity, ID, DTO]) create(w http.ResponseWriter, r *http.Request) {
	if !h.useBeforeHook(h.Operations.Create.BeforeHook, w, r) {
		return
	}

	creator, ok := h.Resource.(crud.Creator[Entity])
	if !ok {
		h.errMethodNotAllowed(w, r)
		return
	}
	defer r.Body.Close()

	bodyReadLimit := h.getBodyReadLimit()
	bytes, err := io.ReadAll(io.LimitReader(r.Body, bodyReadLimit))
	if err != nil {
		h.getErrorHandler().HandleError(w, r, err)
		return
	}

	var dto DTO
	if err := json.Unmarshal(bytes, &dto); err != nil {
		if int(bodyReadLimit) == len(bytes) {
			h.getErrorHandler().HandleError(w, r,
				ErrRequestEntityTooLarge.With().
					Detailf("Body Limit: %d", bodyReadLimit))
			return
		}

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

	if err := json.Unmarshal(bytes, &dto); err != nil {
		h.errInternalServerError(w, r, err)
		return
	}

	if id, ok := h.Mapping.LookupID(entity); ok {
		_, found, err := h.Resource.FindByID(ctx, id)
		if err != nil {
			h.getErrorHandler().HandleError(w, r, err)
			return
		}
		if found {
			h.getErrorHandler().HandleError(w, r, ErrEntityAlreadyExist)
			return
		}
	}

	if err := creator.Create(ctx, &entity); err != nil {
		if errors.Is(err, crud.ErrAlreadyExists) {
			err = ErrEntityAlreadyExist.With().Wrap(err)
		}
		h.getErrorHandler().HandleError(w, r, err)
		return
	}

	dto, err = h.Mapping.MapDTO(ctx, entity)
	if err != nil {
		h.errInternalServerError(w, r, err)
		return
	}

	h.writeJSON(w, r, http.StatusCreated, dto)
}

var DefaultBodyReadLimit int64 = 256 * 1024 * 1024

func (h Handler[Entity, ID, DTO]) getBodyReadLimit() int64 {
	if h.BodyReadLimit != 0 {
		return h.BodyReadLimit
	}
	return DefaultBodyReadLimit
}

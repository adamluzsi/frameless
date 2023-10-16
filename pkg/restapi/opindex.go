package restapi

import (
	"encoding/json"
	"net/http"

	"go.llib.dev/frameless/ports/crud"
	"go.llib.dev/frameless/ports/iterators"
)

type IndexOperation[Entity, ID, DTO any] struct {
	BeforeHook BeforeHook
	Override   func(r *http.Request) iterators.Iterator[Entity]
}

func (ctrl IndexOperation[Entity, ID, DTO]) handle(h Handler[Entity, ID, DTO], r *http.Request) (iterators.Iterator[Entity], bool) {
	if ctrl.Override != nil {
		return ctrl.Override(r), true
	}

	finder, ok := h.Resource.(crud.AllFinder[Entity])
	if !ok {
		return nil, false
	}

	return finder.FindAll(r.Context()), true
}

func (h Handler[Entity, ID, DTO]) index(w http.ResponseWriter, r *http.Request) {
	if !h.useBeforeHook(h.Operations.Index.BeforeHook, w, r) {
		return
	}
	iter, ok := h.Operations.Index.handle(h, r)
	if !ok {
		h.errMethodNotAllowed(w, r)
		return
	}
	h.writeIterJSON(w, r, iter)
}

func (h Handler[Entity, ID, DTO]) writeIterJSON(w http.ResponseWriter, r *http.Request, iter iterators.Iterator[Entity]) {
	if err := iter.Err(); err != nil {
		h.getErrorHandler().HandleError(w, r, err)
		return
	}
	defer func() { _ = iter.Close() }()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if _, err := w.Write([]byte("[")); err != nil {
		return
	}
	if iter.Next() {
		v := iter.Value()

		bytes, err := json.Marshal(&v)
		if err != nil {
			return
		}
		if _, err := w.Write(bytes); err != nil {
			return
		}
	}
	for iter.Next() {
		v := iter.Value()

		bytes, err := json.Marshal(&v)
		if err != nil {
			h.errInternalServerError(w, r, err)
		}
		if _, err := w.Write([]byte(`,`)); err != nil {

		}
		if _, err := w.Write(bytes); err != nil {
			return
		}
	}
	if _, err := w.Write([]byte(`]`)); err != nil {
		return
	}
	_, _ = w.Write([]byte("\n"))
}

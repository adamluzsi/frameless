package rest

import (
	"encoding/json"
	"github.com/adamluzsi/frameless/ports/crud"
	"github.com/adamluzsi/frameless/ports/iterators"
	"net/http"
)

func (h Handler[Entity, ID, DTO]) index(w http.ResponseWriter, r *http.Request) {
	finder, ok := h.Resource.(crud.AllFinder[Entity, ID])
	if !ok {
		h.errMethodNotAllowed(w, r)
		return
	}

	iter := finder.FindAll(r.Context())
	if err := iter.Err(); err != nil {
		h.errInternalServerError(w, r, err)
		return
	}
	defer iter.Close()

	h.writeIterJSON(w, r, iter)
}

func (h Handler[Entity, ID, DTO]) writeIterJSON(w http.ResponseWriter, r *http.Request, iter iterators.Iterator[Entity]) {
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

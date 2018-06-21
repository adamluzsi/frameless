package adapters

import (
	"io"
	"net/http"

	"github.com/adamluzsi/frameless"
	fhttprouter "github.com/adamluzsi/frameless/controllers/adapters/integrations/github.com/julienschmidt/httprouter"
	"github.com/julienschmidt/httprouter"
)

// HTTPRouter create adapter function that fits to Julien Schmidt httprouter common interface
func HTTPRouter(
	controller frameless.Controller,
	buildPresenter frameless.PresenterBuilder,
	buildIterator func(io.Reader) frameless.Iterator,

) func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	closer := func(c io.Closer) {
		if c != nil {
			c.Close()
		}
	}

	return func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		defer closer(r.Body)

		presenter := buildPresenter(w)
		request := fhttprouter.NewRequest(r, buildIterator, p)

		if err := controller.Serve(presenter, request); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}

	}
}

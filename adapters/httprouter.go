package adapters

import (
	"io"
	"net/http"

	"github.com/adamluzsi/frameless/controllers"
	"github.com/adamluzsi/frameless/dataproviders"
	fhttprouter "github.com/adamluzsi/frameless/integrations/github.com/julienschmidt/httprouter"
	"github.com/adamluzsi/frameless/presenters"
	"github.com/julienschmidt/httprouter"
)

// HTTPRouter create adapter function that fits to Julien Schmidt httprouter common interface
func HTTPRouter(
	controller controllers.Controller,
	buildPresenter presenters.PresenterBuilder,
	buildIterator dataproviders.IteratorBuilder,

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

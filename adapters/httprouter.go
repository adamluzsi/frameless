package adapters

import (
	"io"
	"net/http"

	"github.com/adamluzsi/frameless/controllers"
	fhttprouter "github.com/adamluzsi/frameless/integrations/github.com/julienschmidt/httprouter"
	"github.com/julienschmidt/httprouter"
)

// HTTPRouter create adapter function that fits to Julien Schmidt httprouter common interface
func (a *Adapter) HTTPRouter(controller controllers.Controller) func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	closer := func(c io.Closer) {
		if c != nil {
			c.Close()
		}
	}

	return func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		defer closer(r.Body)

		presenter := a.buildPresenter(w)
		request := fhttprouter.NewRequest(r, a.buildIterator, p)

		if err := controller.Serve(presenter, request); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}

	}
}

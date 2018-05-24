package adapters

import (
	"io"
	"net/http"

	"github.com/adamluzsi/frameless/controllers"
	fhttp "github.com/adamluzsi/frameless/integrations/net/http"
)

func (a *Adapter) NetHTTP(controller controllers.Controller) http.Handler {
	closer := func(c io.Closer) {
		if c != nil {
			c.Close()
		}
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer closer(r.Body)

		presenter := a.buildPresenter(w)
		request := fhttp.NewRequest(r, a.buildIterator)

		if err := controller.Serve(presenter, request); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}

	})
}

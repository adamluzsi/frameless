package adapters

import (
	"io"
	"net/http"

	"github.com/adamluzsi/frameless/controllers"
	"github.com/adamluzsi/frameless/dataproviders"
	fhttp "github.com/adamluzsi/frameless/integrations/net/http"
	"github.com/adamluzsi/frameless/presenters"
)

// NetHTTP creates an adapter http.Hander object that can be given to a http.ServerMux
func NetHTTP(
	controller controllers.Controller,
	buildPresenter presenters.PresenterBuilder,
	buildIterator dataproviders.IteratorBuilder,

) http.Handler {

	closer := func(c io.Closer) {
		if c != nil {
			c.Close()
		}
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer closer(r.Body)

		presenter := buildPresenter(w)
		request := fhttp.NewRequest(r, buildIterator)

		if err := controller.Serve(presenter, request); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}

	})
}

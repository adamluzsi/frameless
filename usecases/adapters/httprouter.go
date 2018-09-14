package adapters

import (
	"io"
	"net/http"

	"github.com/adamluzsi/frameless"
	fhttprouter "github.com/adamluzsi/frameless/usecases/adapters/integrations/github.com/julienschmidt/httprouter"
	"github.com/julienschmidt/httprouter"
)

// HTTPRouter create adapter function that fits to Julien Schmidt httprouter common interface
func HTTPRouter(
	useCase frameless.UseCase,
	buildPresenter func(io.Writer) frameless.Presenter,
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

		if err := useCase.Do(request, presenter); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}

	}
}

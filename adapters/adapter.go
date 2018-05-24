package adapters

import (
	"github.com/adamluzsi/frameless/dataproviders"
	"github.com/adamluzsi/frameless/presenters"
)

type Adapter struct {
	buildPresenter presenters.PresenterBuilder
	buildIterator  dataproviders.IteratorBuilder
	options        *options
}

func New(p presenters.PresenterBuilder, b dataproviders.IteratorBuilder, os ...Option) *Adapter {
	return &Adapter{
		buildIterator:  b,
		buildPresenter: p,
		options:        setupOptions(os),
	}
}

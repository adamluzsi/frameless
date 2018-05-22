package controller

import (
	"github.com/adamluzsi/frameless/presenter"
	"github.com/adamluzsi/frameless/request"
)

type Read interface {
	Index
	Show
}

type Index interface {
	Index(presenter.Presenter, request.Request)
}

type Show interface {
	Show(presenter.Presenter, request.Request)
}

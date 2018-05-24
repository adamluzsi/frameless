package controllers

import (
	"github.com/adamluzsi/frameless/presenters"
	"github.com/adamluzsi/frameless/requests"
)

type Controller interface {
	Serve(presenters.Presenter, requests.Request) error
}

type ControllerFunc func(presenters.Presenter, requests.Request) error

func (this ControllerFunc) Serve(p presenters.Presenter, r requests.Request) error {
	return this(p, r)
}

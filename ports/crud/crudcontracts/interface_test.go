package crudcontracts

import (
	"github.com/adamluzsi/frameless/internal/suites"
)

type (
	EntType struct{ ID IDType }
	IDType  struct{}
)

var _ = []suites.Suite{
	Creator[EntType, IDType](nil),
	Finder[EntType, IDType](nil),
	QueryOne[EntType, IDType](nil),
	Updater[EntType, IDType](nil),
	Saver[EntType, IDType](nil),
	Deleter[EntType, IDType](nil),
	OnePhaseCommitProtocol[EntType, IDType](nil),
}

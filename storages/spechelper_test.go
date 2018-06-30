package storages_test

import (
	randomdata "github.com/Pallinder/go-randomdata"
	"github.com/adamluzsi/frameless"
)

type SampleEntity struct {
	ID   string
	Name string
}

func NewEntityForTest(Type frameless.Entity) (NewUniqEntity frameless.Entity) {
	switch Type.(type) {
	case SampleEntity:
		return &SampleEntity{Name: randomdata.SillyName()}

	default:
		panic("NotImplemented")

	}

}

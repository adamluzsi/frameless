package memorystorage

import (
	"math/rand"

	randomdata "github.com/Pallinder/go-randomdata"
	"github.com/adamluzsi/frameless"
)

type SampleEntity struct {
	ID   string
	Name string
}

var SillyNames []string

func init() {
	for index := 0; index < randomdata.Number(100, 200); index++ {
		SillyNames = append(SillyNames, randomdata.SillyName())
	}
}

func NewEntityForTest(Type frameless.Entity) (NewUniqEntity frameless.Entity) {
	switch Type.(type) {
	case SampleEntity:
		return &SampleEntity{Name: SillyNames[rand.Intn(len(SillyNames)-1)]}

	default:
		panic("NotImplemented")

	}

}

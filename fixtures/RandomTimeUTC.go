package fixtures

import (
	"math/rand"
	"time"
)

func RandomTimeUTC() time.Time {
	loc, err := time.LoadLocation(`UTC`)
	if err != nil {
		panic(err)
	}

	base := time.Now().UTC().Add(time.Duration(rand.Int()) * time.Hour).In(loc)
	t, err := time.ParseInLocation(time.RFC3339, base.Format(time.RFC3339), loc)

	if err != nil {
		panic(err)
	}

	return t
}

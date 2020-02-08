package fixtures

import (
	"math/rand"
	"time"
)

var randomSource = rand.NewSource(time.Now().Unix())

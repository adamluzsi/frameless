package memory

import "github.com/adamluzsi/testcase/random"

func toSlice[Entity any, key comparable](m map[key]Entity) []Entity {
	list := make([]Entity, 0, len(m))
	for _, ent := range m {
		list = append(list, ent)
	}
	return list
}

var rnd *random.Random = random.New(random.CryptoSeed{})

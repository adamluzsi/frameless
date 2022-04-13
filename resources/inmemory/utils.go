package inmemory

func toSlice[Ent any, key comparable](m map[key]Ent) []Ent {
	list := make([]Ent, 0, len(m))
	for _, ent := range m {
		list = append(list, ent)
	}
	return list
}

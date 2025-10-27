package datastruct

// Has is an alias for OrderedSet#Contains
//
// Deprecated: use OrderedSet#Contains
func (s OrderedSet[T]) Has(v T) bool {
	return s.Contains(v)
}

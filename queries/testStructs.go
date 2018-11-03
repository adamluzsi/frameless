package queries

type unexportedEntity struct {
	ExtID string `ext:"ID"`
	Data  string
}

type entityWithoutIDField struct {
	Data string
}

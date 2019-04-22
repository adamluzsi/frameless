package specs

type ExportedEntity struct {
	ExtID string `ext:"ID"`
	Data  string
}

type unexportedEntity struct {
	ExtID string `ext:"ID"`
	Data  string
}

type entityWithoutIDField struct {
	Data string
}


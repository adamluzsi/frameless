package resources_test

type IDByIDField struct {
	ID string
}

type IDByTag struct {
	DI string `ext:"ID"`
}

type IDAsInterface struct {
	ID interface{} `ext:"ID"`
}

type IDAsPointer struct {
	ID *string `ext:"ID"`
}

type UnidentifiableID struct {
	UserID string
}

package testhelper

type IDByIDField struct {
	ID string
}

type IDByUppercaseTag struct {
	DI string `ext:"ID"`
}

type IDByLowercaseTag struct {
	DI string `ext:"id"`
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

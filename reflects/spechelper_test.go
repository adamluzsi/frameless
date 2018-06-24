package reflects_test

type IDInFieldName struct {
	ID string
}

type IDInTagName struct {
	DI string `storage:"ID"`
}

type UnidentifiableID struct {
	UserID string
}

type InterfaceObject interface{}

type StructObject struct{}

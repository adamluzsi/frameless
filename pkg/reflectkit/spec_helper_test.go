package reflectkit_test

type IDInFieldName struct {
	ID string
}

type IDInTagName struct {
	DI string `ext:"ID"`
}

type UnidentifiableID struct {
	UserID string
}

type InterfaceObject interface{}

type StructObject struct{}

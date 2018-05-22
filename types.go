package frameless

// Types here only exists to make interface definitions verbose in term of intention

// BusinessEntity represents an object that is specified for a given project
type BusinessEntity interface{}

// Primitive represent an object that is belongs to the core types
//	such as string or int for example
type Primitive interface{}

// Content is a object that could include BusinessEntities and Primitives
// as well and usually served to some kind of presenter layer
type Content interface{}

// ExternalResource represent an object that created by an external resource such as http request or an io
type ExternalResource interface{}

package frameless

const ErrNotImplemented Error = "ErrNotImplemented"

const ErrNotFound Error = "ErrNotFound"

const ErrIDRequired Error = `
Can't find the ID in the current structure
if there is no ID in the subject structure
custom test needed that explicitly defines how ID is stored and retried from an entity
`

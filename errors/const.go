package errors

import "github.com/adamluzsi/frameless"

const ErrNotImplemented frameless.Error = "ErrNotImplemented"

const ErrNotFound frameless.Error = "ErrNotFound"

const ErrIDRequired frameless.Error = `
Can't find the ID in the current structure
if there is no ID in the subject structure
custom test needed that explicitly defines how ID is stored and retried from an entity
`

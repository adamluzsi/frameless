package queryerrors

import "github.com/adamluzsi/frameless/errtype"

const ErrNotFound errtype.Error = "ErrNotFound"

const ErrIDRequired errtype.Error = `
Can't find the ID in the current structure
if there is no ID in the subject structure
custom test needed that explicitly defines how ID is stored and retrived from an entity
`
package queryerrors

import "github.com/adamluzsi/frameless/errstr"

const ErrNotFound errstr.Error = "ErrNotFound"

const ErrIDRequired errstr.Error = `
Can't find the ID in the current structure
if there is no ID in the subject structure
custom test needed that explicitly defines how ID is stored and retrived from an entity
`
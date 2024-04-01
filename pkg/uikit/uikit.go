package uikit

import "io"

type Renderer interface {
	Render(w io.Writer) error
}

type Component struct {
}

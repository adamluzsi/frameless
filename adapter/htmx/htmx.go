package htmx

import "go.llib.dev/frameless/pkg/httpkit"

type App struct {
	MountPoint string
}

func (app App) Mount(m httpkit.Multiplexer, pattern string) {

}

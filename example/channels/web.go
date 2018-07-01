package channels

import (
	"html/template"
	"io"

	"github.com/adamluzsi/frameless/iterators/iterateover"

	"net/http"

	"github.com/adamluzsi/frameless/controllers/adapters"
	"github.com/adamluzsi/frameless/example"

	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/example/usecases"
)

func NewHTTPMux(usecases *usecases.UseCases) *http.ServeMux {
	return (&WEB{usecases: usecases}).toServerMux()
}

type WEB struct {
	usecases *usecases.UseCases
}

func (web *WEB) toServerMux() *http.ServeMux {
	mux := http.NewServeMux()

	add := adapters.NetHTTP(
		frameless.ControllerFunc(web.usecases.AddNote),
		func(w io.Writer) frameless.Presenter { return web.presentNote(w) },
		func(r io.Reader) frameless.Iterator { return iterateover.LineByLine(r) },
	)

	mux.Handle("/add", add)

	list := adapters.NetHTTP(
		frameless.ControllerFunc(web.usecases.ListNotes),
		func(w io.Writer) frameless.Presenter { return web.presentNotes(w) },
		func(r io.Reader) frameless.Iterator { return iterateover.LineByLine(r) },
	)

	mux.Handle("/list", list)

	return mux
}

var notesTemplateText = `
<table>
  <tr>
    <th>ID</th>
    <th>Title</th>
    <th>Content</th>
  </tr>
  {{range .}}
  <tr>
    <td>{{.ID}}</td>
    <td>{{.Title}}</td>
    <td>{{.Content}}</td>
  </tr>
  {{end}}
</table>
`

var notesTemplate = template.Must(template.New("present-note-list").Parse(notesTemplateText))

func (web *WEB) presentNote(w io.Writer) frameless.Presenter {
	return frameless.PresenterFunc(func(message interface{}) error {
		note := message.(*example.Note)
		notes := []*example.Note{note}
		return web.executeNotesTemplate(w, notes)
	})
}

func (web *WEB) presentNotes(w io.Writer) frameless.Presenter {
	return frameless.PresenterFunc(func(message interface{}) error {
		notes := message.([]*example.Note)

		return web.executeNotesTemplate(w, notes)
	})
}

func (web *WEB) executeNotesTemplate(w io.Writer, message interface{}) error {
	notes := message.([]*example.Note)

	return notesTemplate.Execute(w, notes)
}

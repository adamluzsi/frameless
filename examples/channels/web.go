package channels

import (
	"html/template"
	"io"

	"github.com/adamluzsi/frameless/iterators/iterateover"

	"net/http"

	"github.com/adamluzsi/frameless/usecases/adapters"
	"github.com/adamluzsi/frameless/examples"

	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/examples/usecases"
)

func NewHTTPHandler(usecases *usecases.UseCases) http.Handler {
	return (&builder{usecases: usecases}).toServerMux()
}

type builder struct {
	usecases *usecases.UseCases
}

func (b *builder) toServerMux() *http.ServeMux {
	mux := http.NewServeMux()

	add := adapters.NetHTTP(
		frameless.UseCaseFunc(b.usecases.AddNote),
		func(w io.Writer) frameless.Encoder { return b.presentNote(w) },
		func(r io.Reader) frameless.Iterator { return iterateover.LineByLine(r) },
	)

	mux.Handle("/add", add)

	list := adapters.NetHTTP(
		frameless.UseCaseFunc(b.usecases.ListNotes),
		func(w io.Writer) frameless.Encoder { return b.presentNotes(w) },
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

func (b *builder) presentNote(w io.Writer) frameless.Encoder {
	return frameless.EncoderFunc(func(message interface{}) error {
		note := message.(*examples.Note)
		notes := []*examples.Note{note}
		return b.executeNotesTemplate(w, notes)
	})
}

func (b *builder) presentNotes(w io.Writer) frameless.Encoder {
	return frameless.EncoderFunc(func(message interface{}) error {
		notes := message.([]*examples.Note)

		return b.executeNotesTemplate(w, notes)
	})
}

func (b *builder) executeNotesTemplate(w io.Writer, message interface{}) error {
	notes := message.([]*examples.Note)

	return notesTemplate.Execute(w, notes)
}

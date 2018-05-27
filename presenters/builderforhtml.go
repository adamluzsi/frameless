package presenters

import (
	"bytes"
	"html/template"
	"io"

	"github.com/adamluzsi/frameless"
)

// ForHTMLTemplate creates a builder func that than could be provided for example to a adapter that handles a external interface
func ForHTMLTemplate(ts ...*template.Template) frameless.PresenterBuilder {
	return frameless.PresenterBuilder(func(w io.Writer) frameless.Presenter {
		return frameless.PresenterFunc(func(data interface{}) error {

			mostInnerTemplate := ts[len(ts)-1]
			tContent := bytes.NewBuffer([]byte{})

			if err := mostInnerTemplate.Execute(tContent, data); err != nil {
				return err
			}

			rest := ts[:len(ts)-1]
			currentContent := tContent.String()

			for i := len(rest) - 1; i >= 0; i-- {
				t := rest[i]
				b := bytes.NewBuffer([]byte{})

				if err := t.Execute(b, template.HTML(currentContent)); err != nil {
					return err
				}

				currentContent = b.String()
			}

			w.Write([]byte(currentContent))

			return nil

		})
	})
}

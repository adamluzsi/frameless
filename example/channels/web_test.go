package channels_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/adamluzsi/frameless/iterators"
	"github.com/adamluzsi/frameless/queries"

	randomdata "github.com/Pallinder/go-randomdata"
	"github.com/adamluzsi/frameless/example"
	"github.com/stretchr/testify/require"

	"github.com/adamluzsi/frameless"

	"github.com/adamluzsi/frameless/storages"

	"github.com/adamluzsi/frameless/example/channels"
	"github.com/adamluzsi/frameless/example/usecases"
)

func ExampleNewHTTPHandler() (http.Handler, frameless.Storage) {
	s := storages.NewMemory()
	u := usecases.NewUseCases(s)
	return channels.NewHTTPHandler(u), s
}

func NewSampleNote() *example.Note {
	return &example.Note{
		Title:   randomdata.SillyName(),
		Content: randomdata.SillyName(),
	}
}

func TestNewHTTPHandler(t *testing.T) {
	t.Run("GET /list", func(t *testing.T) {

		t.Run("when no notes in the storage", func(t *testing.T) {
			t.Parallel()

			h, _ := ExampleNewHTTPHandler()
			w := httptest.NewRecorder()
			r := httptest.NewRequest("GET", "/list", strings.NewReader(``))

			h.ServeHTTP(w, r)

			body := w.Body.String()
			require.NotContains(t, body, `<td>`)

		})

		t.Run("when note stored in the storage", func(t *testing.T) {
			note := NewSampleNote()

			t.Parallel()

			h, s := ExampleNewHTTPHandler()
			w := httptest.NewRecorder()
			r := httptest.NewRequest("GET", "/list", strings.NewReader(``))

			require.Nil(t, s.Create(note))
			h.ServeHTTP(w, r)

			body := w.Body.String()
			require.Contains(t, body, note.ID)
			require.Contains(t, body, note.Title)
			require.Contains(t, body, note.Content)

		})

	})

	t.Run("POST /add", func(t *testing.T) {
		sample := NewSampleNote()

		t.Parallel()

		h, s := ExampleNewHTTPHandler()
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", fmt.Sprintf("/add?Title=%s&Content=%s", sample.Title, sample.Content), strings.NewReader(``))

		h.ServeHTTP(w, r)

		rgx := regexp.MustCompile(`<td>([^<]+)</td>`)
		matches := rgx.FindAllStringSubmatch(w.Body.String(), -1)
		require.True(t, len(matches) > 0)

		var note example.Note
		if err := iterators.First(s.Find(queries.ByID{Type: note, ID: matches[0][1]}), &note); err != nil {
			t.Fatal(err)
		}

		require.Equal(t, sample.Title, note.Title)
		require.Equal(t, sample.Content, note.Content)

	})
}

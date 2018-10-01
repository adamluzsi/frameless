package channels

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/adamluzsi/frameless/examples"

	"github.com/adamluzsi/frameless/iterators"
	"github.com/adamluzsi/frameless/requests"

	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/examples/usecases"
)

type HTTPServer interface {
	ListenAndServe() error
}

func NewCLI(out io.Writer, usecases *usecases.UseCases, server HTTPServer) *CLI {
	return &CLI{
		usecases: usecases,
		server:   server,
		writer:   out,
	}
}

type CLI struct {
	usecases *usecases.UseCases
	server   HTTPServer
	writer   io.Writer
}

func (cli *CLI) Run(args []string) error {

	if len(args) == 0 {
		args = append(args, "unknown")
	}

	switch args[0] {
	case "add":
		return cli.addNote(args[1:])

	case "list":
		return cli.listNotes(args[1:])

	case "http":
		fmt.Fprintln(cli.writer, "Listen And Do on :8080")
		return cli.server.ListenAndServe()

	default:
		fmt.Println("use one of the following commands: add, list, http")
		os.Exit(1)

	}

	return nil
}

func (cli *CLI) addNote(args []string) error {
	f := flag.NewFlagSet("add", 1)
	title := f.String("t", "", "Title of the note")

	content := f.String("c", "", "Content of the note")

	if err := f.Parse(args); err != nil {
		return err
	}

	if *title == "" {
		return cli.error("missing title option")
	}

	if *content == "" {
		return cli.error("missing content option")
	}

	ctx := context.Background()
	ctx = context.WithValue(ctx, "Title", *title)
	ctx = context.WithValue(ctx, "Content", *content)
	r := requests.New(ctx, iterators.NewEmpty())

	return cli.usecases.AddNote(r, frameless.PresenterFunc(cli.presentNote))

}

func (cli *CLI) presentNote(message interface{}) error {
	note, ok := message.(*examples.Note)

	if !ok {
		return errors.New("contract violation")
	}

	_, err := fmt.Fprintf(cli.writer, "\nNote created\n\n\tID:\t\t%s\n\tTitle:\t\t%s\n\tContent:\t\t%s\n", note.ID, note.Title, note.Content)

	return err
}

func (cli *CLI) listNotes(args []string) error {
	f := flag.NewFlagSet("list", 1)

	if err := f.Parse(args); err != nil {
		return err
	}

	r := requests.New(context.Background(), iterators.NewEmpty())

	return cli.usecases.ListNotes(r, frameless.PresenterFunc(cli.presentNoteList))
}

func (cli *CLI) presentNoteList(message interface{}) error {
	notes, ok := message.([]*examples.Note)

	if !ok {
		return errors.New("contract violation")
	}

	_, err := fmt.Fprintf(cli.writer, "\t%s\t\t%s\t\t%s\n", "ID", "Title", "Content")

	if err != nil {
		return err
	}

	for _, note := range notes {
		_, err := fmt.Fprintf(cli.writer, "\t%s\t\t%s\t\t%s\n", note.ID, note.Title, note.Content)

		if err != nil {
			return err
		}
	}

	return nil
}

func (cli *CLI) error(msg string) error {
	fmt.Fprintln(cli.writer, msg)
	return errors.New(msg)
}

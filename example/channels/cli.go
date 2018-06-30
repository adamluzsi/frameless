package channels

import (
	"fmt"
	"os"

	"github.com/adamluzsi/frameless"
)

func NewCLI(storage frameless.Storage) *CLI {
	return &CLI{storage: storage}
}

type CLI struct {
	storage frameless.Storage
}

func (cli *CLI) Run(args []string) error {

	if len(args) == 0 {
		args = append(args, "unknown")
	}

	switch args[0] {
	case "add":

	case "list":

	default:
		fmt.Println("use one of the following commands: add, list")
		os.Exit(1)

	}

	return nil
}

func (cli *CLI) addNote(args []string) error {
	// ucs := usecases.NewUseCases(cli.storage)

	// ucs.AddNote(cli.presentNoteList)

	return nil
}

func (cli *CLI) presentNoteList(message interface{}) error {
	return nil
}

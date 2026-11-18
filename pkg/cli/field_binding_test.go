package cli_test

import (
	"testing"

	"go.llib.dev/frameless/pkg/cli"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
)

func TestBuild_BoxedFields(t *testing.T) {
	s := testcase.NewSpec(t)

	type options struct {
		Count  int    `flag:"count" env:"CLI_TEST_BOX_COUNT" default:"1" required:"true"`
		Label  string `flag:"label" default:"inner"`
		Preset string `flag:"preset" default:"unused"`
	}
	type inner struct {
		Options options  `flag:"options"`
		Args    []string `arg:"0:" optional:"true"`
	}
	type outer struct {
		Nested  any
		Sibling string `flag:"sibling" default:"sibling"`
	}
	type command struct {
		cli.HandlerFunc
		Box any
	}
	newCommand := func() *command {
		return &command{Box: outer{Nested: inner{Options: options{Preset: "preset"}}}}
	}

	s.Test("writes through both boxes without losing sibling inputs or changing concrete types", func(t *testcase.T) {
		t.Setenv("CLI_TEST_BOX_COUNT", "3")
		cmd := newCommand()
		_, err := cli.Build(cmd, &cli.Request{Args: []string{`-options={"Label":"outer"}`, "-label=flag", "-count=0", "one", "two"}})
		assert.NoError(t, err)
		assert.Equal(t, cmd.Box, any(outer{Nested: inner{Options: options{Label: "outer", Preset: "preset"}, Args: []string{"one", "two"}}, Sibling: "sibling"}))
	})

	s.Test("env and defaults remain visible when an omitted variadic argument is initialized", func(t *testcase.T) {
		t.Setenv("CLI_TEST_BOX_COUNT", "3")
		cmd := newCommand()
		_, err := cli.Build(cmd, &cli.Request{})
		assert.NoError(t, err)
		assert.Equal(t, cmd.Box, any(outer{Nested: inner{Options: options{Count: 3, Label: "inner", Preset: "preset"}, Args: []string{}}, Sibling: "sibling"}))
	})

	s.Test("usage discovers boxed flags without binding them", func(t *testcase.T) {
		cmd := newCommand()
		usage, err := cli.Usage(cmd, "box")
		assert.NoError(t, err)
		assert.Contains(t, usage, "-count")
		assert.Equal(t, cmd.Box, newCommand().Box)
	})

	s.Test("an explicitly empty child of a zero struct still runs its validator", func(t *testcase.T) {
		type group struct {
			Level VLevel `flag:"level"`
		}
		cmd := &command{Box: struct {
			Group group `flag:"group"`
		}{}}
		_, err := cli.Build(cmd, &cli.Request{Args: []string{"-level="}})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "level can't be empty")
	})

	s.Test("an explicitly zero array element field is checked against its validation tag", func(t *testcase.T) {
		type limits struct {
			Limit int `flag:"limit" min:"1"`
		}
		cmd := &command{Box: [1]limits{}}
		_, err := cli.Build(cmd, &cli.Request{Args: []string{"-limit=0"}})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "-limit")
	})

	s.Test("outer JSON can clear an interface without resurrecting its earlier input", func(t *testcase.T) {
		var cmd struct {
			cli.HandlerFunc
			Config struct{ Box any } `flag:"config"`
		}
		cmd.Config.Box = struct {
			Level VLevel `flag:"level"`
		}{}
		_, err := cli.Build(&cmd, &cli.Request{Args: []string{"-level=", `-config={"Box":null}`}})
		assert.NoError(t, err)
		assert.Nil(t, cmd.Config.Box)
	})
}

package cli_test

import (
	"fmt"
	"testing"

	"go.llib.dev/frameless/pkg/cli"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
)

func TestUsage(t *testing.T) {
	t.Run("struct", func(t *testing.T) {
		usage, err := cli.Usage(CommandE2E{}, "thepath")
		assert.NoError(t, err)

		if testing.Verbose() {
			t.Log(usage)
		}

		testcase.OnFail(t, func() {
			t.Log(usage)
		})

		assert.Contains(t, usage, "Usage: thepath [OPTION]... [Arg1] [Arg2] [Arg3]")
		assert.Contains(t, usage, "-str=[string]: flag1 desc")
		assert.Contains(t, usage, `-strwd=[string] (default: "defval")`)
		assert.Contains(t, usage, "-int=[int]")
		assert.Contains(t, usage, "-bool=[bool]")
		assert.Contains(t, usage, "-sbool=[bool]")
		assert.Contains(t, usage, "-fbool=[bool]")
		assert.Contains(t, usage, "Arg1 [string]")
		assert.Contains(t, usage, "Arg2 [int]")
		assert.Contains(t, usage, "Arg3 [bool]")
		assert.Contains(t, usage, "-int=[int] (env: FLAG3)", "env variable is mentioned")
		assert.Contains(t, usage, "Environments:")
		assert.Contains(t, usage, "ENV1 [string]")
		assert.Contains(t, usage, `default: "1vne"`)
		assert.Contains(t, usage, "ENV2, ENV22 [string]")
		assert.Contains(t, usage, `default: "2vne"`)
	})
	t.Run("when cli.Handler#Usage(path) is supported", func(t *testing.T) {
		usage, err := cli.Usage(CommandWithUsageSupport{}, "thepath")
		assert.NoError(t, err)

		assert.Contains(t, usage, "Custom Usage Message: thepath")
	})
}

type ExampleCommand struct {
	Flag1 string `flag:"str,s" desc:"flag1 desc" env:"FLAG1" required:"true"`
	Flag2 bool   `flag:"bl" default:"defval"`
	Flag3 int    `flag:"i" env:"FLAG3" description:"it is a configuration input, an int one"`

	Arg1 string `arg:"0"`
	Arg2 int    `arg:"1"`
	Arg3 bool   `arg:"2"`

	Val1 string `desc:"descriptive description" flag:"val" arg:"3" env:"VAL1" default:"The answer is 42" required:"true"`
	Env1 string `desc:"env-1" env:"ENV1" default:"1vne"`
	Env2 string `desc:"env-1" env:"ENV2,ENV22" default:"2vne"`

	ExampleConfig1 ExampleConfig1
	ExampleConfig2
}

func (cmd ExampleCommand) ServeCLI(w cli.ResponseWriter, r *cli.Request) {
	w.ExitCode(cli.ExitCodeOK)
	w.Write([]byte("OK"))
}

type ExampleConfig1 struct {
	A string `env:"A"`
	B string `env:"B"`
	C int    `env:"C" default:"42"`
}

type ExampleConfig2 struct {
	A string `env:"A2"`
}

func Test_example(t *testing.T) {
	testcase.GetEnv(t, "example", t.SkipNow)

	var mux cli.Mux
	mux.Handle("cmd", ExampleCommand{})

	var w cli.ResponseRecorder
	mux.ServeCLI(&w, &cli.Request{
		Args: []string{"cmd", "-h"},
	})

	fmt.Println(w.Out.String())
}

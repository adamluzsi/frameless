package cli_test

import (
	"testing"

	"go.llib.dev/frameless/pkg/cli"
	"go.llib.dev/testcase/assert"
)

type requestConfig struct {
	Name   string `flag:"name"`
	Self   *requestConfig
	Labels map[string]string
	Counts []*int
	Calls  int `json:"-"`
}

func (c *requestConfig) MiddlewareCLI(next cli.Handler) cli.Handler {
	c.Calls++
	return next
}

type requestCopyCommand struct {
	Alias   any
	Config  *requestConfig `flag:"config"`
	Service *int
	Opaque  *requestConfig `opaque:""`
	observe func(requestCopyCommand)
}

func (cmd requestCopyCommand) ServeCLI(cli.ResponseWriter, *cli.Request) { cmd.observe(cmd) }

func TestServeCLI_nestedInputCopy(t *testing.T) {
	type config struct {
		Name string `flag:"name"`
	}
	preset := &config{Name: "preset"}
	cmd := struct {
		cli.HandlerFunc
		Config *config
	}{Config: preset}
	cmd.HandlerFunc = func(cli.ResponseWriter, *cli.Request) {}
	var rr cli.ResponseRecorder
	cli.ServeCLI(cmd, &rr, &cli.Request{Args: []string{"-name=changed"}})
	assert.Equal(t, rr.Code, cli.ExitCodeOK)
	assert.Equal(t, preset.Name, "preset")
	cli.ServeCLI(&cmd, &rr, &cli.Request{Args: []string{"-name=in-place"}})
	assert.Equal(t, rr.Code, cli.ExitCodeOK)
	assert.Equal(t, preset.Name, "in-place")
}

func TestServeCLI_requestCopy(t *testing.T) {
	n := 3
	cfg := &requestConfig{Name: "preset", Labels: map[string]string{"key": "old"}, Counts: []*int{&n}}
	cfg.Self = cfg
	cmd := requestCopyCommand{Config: cfg, Alias: cfg, Service: new(int), Opaque: &requestConfig{}}
	var seen []requestCopyCommand
	cmd.observe = func(c requestCopyCommand) { seen = append(seen, c) }
	for _, args := range [][]string{{"-name=first", `-config={"Labels":{"key":"new"},"Counts":[9]}`}, nil} {
		var rr cli.ResponseRecorder
		cli.ServeCLI(cmd, &rr, &cli.Request{Args: args})
		assert.Equal(t, rr.Code, cli.ExitCodeOK, assert.Message(rr.Err.String()))
	}
	assert.Equal(t, len(seen), 2)
	for _, c := range seen {
		assert.True(t, c.Config != cfg && c.Config == c.Alias && c.Config == c.Config.Self)
		assert.True(t, c.Service == cmd.Service && c.Opaque == cmd.Opaque)
		assert.Equal(t, c.Config.Calls, 1)
	}
	assert.Equal(t, seen[0].Config.Name, "first")
	assert.Equal(t, seen[0].Config.Labels["key"], "new")
	assert.Equal(t, *seen[0].Config.Counts[0], 9)
	assert.Equal(t, seen[1].Config.Name, "preset")
	assert.Equal(t, seen[1].Config.Labels["key"], "old")
	assert.Equal(t, *seen[1].Config.Counts[0], 3)
	assert.Equal(t, cfg.Calls, 0)
}

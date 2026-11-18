package cli_test

import (
	"testing"

	"go.llib.dev/frameless/pkg/cli"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
)

func TestBuild_JSONLayers(t *testing.T) {
	s := testcase.NewSpec(t)

	type settings struct {
		Count int    `json:"n" flag:"count" default:"3"`
		Label string `flag:"label" default:"inner"`
	}
	type command struct {
		cli.HandlerFunc
		Settings settings `flag:"settings" default:"{\"n\":0,\"Label\":\"outer\"}"`
	}

	s.Test("a pre-set field prevents the enclosing JSON default but not a sibling default", func(t *testcase.T) {
		cmd := command{Settings: settings{Count: 7}}
		_, err := cli.Build(&cmd, &cli.Request{})
		assert.NoError(t, err)
		assert.Equal(t, cmd.Settings, settings{Count: 7, Label: "inner"})
	})

	s.Test("an inner flag overrides the outer zero default and an outer input merges onto both", func(t *testcase.T) {
		var cmd command
		_, err := cli.Build(&cmd, &cli.Request{Args: []string{`-settings={"Label":""}`, "-count=8"}})
		assert.NoError(t, err)
		assert.Equal(t, cmd.Settings, settings{Count: 8})
	})
}

func TestBuild_JSONPointerValidation(t *testing.T) {
	s := testcase.NewSpec(t)

	type auth struct {
		Token string `flag:"token" required:"true"`
		Mode  string `flag:"mode" enum:"read,write"`
	}
	type command struct {
		cli.HandlerFunc
		Config struct{ Auth *auth } `flag:"config"`
	}
	act := func(args ...string) error {
		var cmd command
		cmd.Config.Auth = &auth{}
		_, err := cli.Build(&cmd, &cli.Request{Args: args})
		return err
	}

	s.Test("clearing a pointer drops its required and loaded fields from validation", func(t *testcase.T) {
		assert.NoError(t, act("-mode=invalid", `-config={"Auth":null}`))
	})

	s.Test("a replacement pointee does not inherit the detached field's input", func(t *testcase.T) {
		// encoding/json processes duplicate keys in order: clear, then allocate again.
		err := act("-token=old", `-config={"Auth":null,"Auth":{}}`)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "missing input for token")
	})

	s.Test("revisiting pre-set map elements does not turn them into supplied inputs", func(t *testcase.T) {
		var cmd struct {
			cli.HandlerFunc
			Presets map[string]auth
		}
		cmd.Presets = map[string]auth{"local": {Token: "old", Mode: "pre-set"}}
		_, err := cli.Build(&cmd, &cli.Request{})
		assert.NoError(t, err)
	})

	s.Test("validation tags apply to non-empty values in a replacement pointee", func(t *testcase.T) {
		err := act(`-config={"Auth":null,"Auth":{"Token":"new","Mode":"invalid"}}`)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "-mode")
	})

	s.Test("a retained pointee's explicitly empty input still counts, but an absent optional field stays absent", func(t *testcase.T) {
		assert.NoError(t, act("-token=", `-config={"Auth":{}}`))
	})

	s.Test("a retained pointee's loaded field is checked after the merge", func(t *testcase.T) {
		err := act("-token=t", "-mode=read", `-config={"Auth":{"Mode":"invalid"}}`)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid")
		assert.Contains(t, err.Error(), "-mode")
	})
}

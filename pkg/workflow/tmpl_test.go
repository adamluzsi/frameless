package workflow_test

import (
	"context"
	"github.com/adamluzsi/frameless/pkg/workflow"
	"github.com/adamluzsi/testcase/assert"
	"testing"
)

func TestTemplate_condition(t *testing.T) {
	var _ workflow.Condition = workflow.Template(``)
	t.Run("when condition is truthy", func(t *testing.T) {
		var vs = workflow.Vars{"x": 42}
		tmpl := workflow.Template("eq .x 42")
		ok, err := tmpl.Check(context.Background(), &vs)
		assert.NoError(t, err)
		assert.True(t, ok)
	})
	t.Run("when condition is falsy", func(t *testing.T) {
		var vs = workflow.Vars{"x": 24}
		tmpl := workflow.Template("eq .x 42")
		ok, err := tmpl.Check(context.Background(), &vs)
		assert.NoError(t, err)
		assert.False(t, ok)
	})
	t.Run("when condition referencing something which is missing", func(t *testing.T) {
		var vs = workflow.Vars{}
		tmpl := workflow.Template("eq .x 42")
		ok, err := tmpl.Check(context.Background(), &vs)
		assert.NoError(t, err)
		assert.False(t, ok)
	})
}

func TestTemplate_task(t *testing.T) {
	var _ workflow.Task = workflow.Template(``)
	var vs = workflow.Vars{"x": 42, "y": 10}
	tmpl := workflow.Template(`var "x" 24` + "\n" + `var "z" (var "y")` + "\n" + `var "y" "hello"` + "\n")
	assert.NoError(t, tmpl.Exec(context.Background(), &vs))
	assert.Equal(t, vs["x"], 24)
	assert.Equal(t, vs["y"], "hello")
	assert.Equal(t, vs["z"], 10)
}

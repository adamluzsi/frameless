package workflow_test

import (
	"context"
	"github.com/adamluzsi/frameless/pkg/workflow"
	"github.com/adamluzsi/testcase/assert"
	"testing"
)

func TestCondTemplate(t *testing.T) {
	t.Run("when condition is truthy", func(t *testing.T) {
		var vs = workflow.Variables{"x": 42}
		ct := workflow.Template("eq .x 42")
		ok, err := ct.Check(context.Background(), &vs)
		assert.NoError(t, err)
		assert.True(t, ok)
	})
	t.Run("when condition is falsy", func(t *testing.T) {
		var vs = workflow.Variables{"x": 24}
		ct := workflow.Template("eq .x 42")
		ok, err := ct.Check(context.Background(), &vs)
		assert.NoError(t, err)
		assert.False(t, ok)
	})
	t.Run("when condition referencing something which is missing", func(t *testing.T) {
		var vs = workflow.Variables{}
		ct := workflow.Template("eq .x 42")
		ok, err := ct.Check(context.Background(), &vs)
		assert.NoError(t, err)
		assert.False(t, ok)
	})
}

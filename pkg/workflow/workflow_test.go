package workflow_test

import (
	"context"
	"fmt"
	wf "github.com/adamluzsi/frameless/pkg/workflow"
	"github.com/adamluzsi/testcase/assert"
	"testing"
	"time"
)

var SampleProcessDefinition = wf.ProcessDefinition{
	Task: wf.Sequence{
		wf.UseParticipant{ID: wf.ParticipantID("42")},
		wf.If{
			Cond: wf.Template(`.x != 42`),
			Then: nil,
		},
		wf.If{
			Cond: wf.Comparison{
				Left:      wf.ConstValue{Value: 42},
				Right:     wf.RefValue{Key: "x"},
				Operation: "!=",
			},
			Then: wf.UseParticipant{ID: "to-do"},
			Else: nil,
		},
		wf.While{
			Cond:  wf.Template(`.x != 42`),
			Block: wf.UseParticipant{ID: "X"},
		},
	},
}

func Test_smoke(t *testing.T) {
	engine := wf.Engine{}

	const fooParticipant = "foo"

	assert.NoError(t, engine.RegisterParticipant(fooParticipant, func(ctx context.Context, val int) (string, error) {
		return fmt.Sprintf("%d", val), nil
	}))

	var out string
	assert.NoError(t, engine.RegisterParticipant("leak", func(v string) { out = v }))

	iid, err := engine.Exec(context.Background(), wf.ProcessDefinition{
		Task: wf.Sequence{
			wf.UseParticipant{
				ID: fooParticipant,
				Args: []wf.Value{
					wf.ConstValue{Value: 42},
					wf.ConstValue{Value: "forty-two"},
				},
				Out: []wf.VariableKey{
					"foo-res",
				},
			},
			wf.If{
				Cond: wf.Comparison{
					Left:      wf.RefValue{Key: "foo-res"},
					Right:     wf.ConstValue{Value: "42"},
					Operation: "==",
				},
				Then: wf.UseParticipant{
					ID: "leak",
					Args: []wf.Value{
						wf.RefValue{Key: "foo-res"},
					},
				},
			},
		},
	})
	assert.NoError(t, err)
	assert.NotEmpty(t, iid)

	assert.EventuallyWithin(5*time.Second).Assert(t, func(it assert.It) {
		it.Must.Equal(out, "42")
	})
}

package workflow_test

import (
	"context"
	"github.com/adamluzsi/frameless/pkg/workflow"
	"github.com/adamluzsi/testcase"
	"testing"
)

func TestComparison(t *testing.T) {
	type TC struct {
		Left, Right workflow.Expression
		Operation   string
		IsEqual     bool
		HasError    error
		Vars        *workflow.Variables
	}
	cases := map[string]TC{
		"op ==; equal": TC{
			Left:      workflow.ConstValue{Value: 42},
			Right:     workflow.ConstValue{Value: 42},
			Operation: "==",
			IsEqual:   true,
			HasError:  nil,
		},
		"op ==; not equal": TC{
			Left:      workflow.ConstValue{Value: 42},
			Right:     workflow.ConstValue{Value: 24},
			Operation: "==",
			IsEqual:   false,
			HasError:  nil,
		},
		"op !=; equal": TC{
			Left:      workflow.ConstValue{Value: 42},
			Right:     workflow.ConstValue{Value: 42},
			Operation: "!=",
			IsEqual:   false,
			HasError:  nil,
		},
		"op !=; not equal": TC{
			Left:      workflow.ConstValue{Value: 42},
			Right:     workflow.ConstValue{Value: 24},
			Operation: "!=",
			IsEqual:   true,
			HasError:  nil,
		},
	}
	testcase.TableTest(t, cases, func(t *testcase.T, tc TC) {
		var cond workflow.Condition = workflow.Comparison{
			Left:      tc.Left,
			Right:     tc.Right,
			Operation: tc.Operation,
		}
		ok, err := cond.Check(context.Background(), tc.Vars)
		if tc.HasError != nil {
			t.Must.ErrorIs(err, tc.HasError)
		} else {
			t.Must.NoError(err)
			t.Must.Equal(ok, tc.IsEqual)
		}
	})
}

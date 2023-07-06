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
		IsOK        bool
		HasError    error
		Vars        *workflow.Variables
	}
	cases := map[string]TC{
		// string 
		"op ==; OK ; string": {
			Left:      workflow.ConstValue{Value: "42"},
			Right:     workflow.ConstValue{Value: "42"},
			Operation: "==",
			IsOK:      true,
			HasError:  nil,
		},
		"op ==; not OK ; string": {
			Left:      workflow.ConstValue{Value: "42"},
			Right:     workflow.ConstValue{Value: "24"},
			Operation: "==",
			IsOK:      false,
			HasError:  nil,
		},
		"op !=; OK ; string": {
			Left:      workflow.ConstValue{Value: "24"},
			Right:     workflow.ConstValue{Value: "42"},
			Operation: "!=",
			IsOK:      true,
			HasError:  nil,
		},
		"op !=; not OK ; string": {
			Left:      workflow.ConstValue{Value: "42"},
			Right:     workflow.ConstValue{Value: "42"},
			Operation: "!=",
			IsOK:      false,
			HasError:  nil,
		},
		// int
		"op ==; OK ; int": {
			Left:      workflow.ConstValue{Value: 42},
			Right:     workflow.ConstValue{Value: 42},
			Operation: "==",
			IsOK:      true,
			HasError:  nil,
		},
		"op ==; not OK ; int": {
			Left:      workflow.ConstValue{Value: 42},
			Right:     workflow.ConstValue{Value: 24},
			Operation: "==",
			IsOK:      false,
			HasError:  nil,
		},
		"op !=; OK ; int": {
			Left:      workflow.ConstValue{Value: 24},
			Right:     workflow.ConstValue{Value: 42},
			Operation: "!=",
			IsOK:      true,
			HasError:  nil,
		},
		"op !=; not OK ; int": {
			Left:      workflow.ConstValue{Value: 42},
			Right:     workflow.ConstValue{Value: 42},
			Operation: "!=",
			IsOK:      false,
			HasError:  nil,
		},
		"op <; OK ; int": {
			Left:      workflow.ConstValue{Value: 24},
			Right:     workflow.ConstValue{Value: 42},
			Operation: "<",
			IsOK:      true,
			HasError:  nil,
		},
		"op <; not OK; int": {
			Left:      workflow.ConstValue{Value: 42},
			Right:     workflow.ConstValue{Value: 24},
			Operation: "<",
			IsOK:      false,
			HasError:  nil,
		},
		"op >; OK ; int": {
			Left:      workflow.ConstValue{Value: 42},
			Right:     workflow.ConstValue{Value: 24},
			Operation: ">",
			IsOK:      true,
			HasError:  nil,
		},
		"op >; not OK; int": {
			Left:      workflow.ConstValue{Value: 24},
			Right:     workflow.ConstValue{Value: 42},
			Operation: ">",
			IsOK:      false,
			HasError:  nil,
		},
		"op <=; OK - less; int": {
			Left:      workflow.ConstValue{Value: 24},
			Right:     workflow.ConstValue{Value: 42},
			Operation: "<=",
			IsOK:      true,
			HasError:  nil,
		},
		"op <=; OK - eq; int": {
			Left:      workflow.ConstValue{Value: 42},
			Right:     workflow.ConstValue{Value: 42},
			Operation: "<=",
			IsOK:      true,
			HasError:  nil,
		},
		"op <=; not OK; int": {
			Left:      workflow.ConstValue{Value: 42},
			Right:     workflow.ConstValue{Value: 24},
			Operation: "<=",
			IsOK:      false,
			HasError:  nil,
		},
		"op >=; OK - less; int": {
			Left:      workflow.ConstValue{Value: 42},
			Right:     workflow.ConstValue{Value: 24},
			Operation: ">=",
			IsOK:      true,
			HasError:  nil,
		},
		"op >=; OK - eq; int": {
			Left:      workflow.ConstValue{Value: 42},
			Right:     workflow.ConstValue{Value: 42},
			Operation: ">=",
			IsOK:      true,
			HasError:  nil,
		},
		"op >=; not OK; int": {
			Left:      workflow.ConstValue{Value: 24},
			Right:     workflow.ConstValue{Value: 42},
			Operation: ">=",
			IsOK:      false,
			HasError:  nil,
		},
		// FLOAT
		"op ==; OK ; float": {
			Left:      workflow.ConstValue{Value: 42.13},
			Right:     workflow.ConstValue{Value: 42.13},
			Operation: "==",
			IsOK:      true,
			HasError:  nil,
		},
		"op ==; not OK ; float": {
			Left:      workflow.ConstValue{Value: 42.13},
			Right:     workflow.ConstValue{Value: 24.13},
			Operation: "==",
			IsOK:      false,
			HasError:  nil,
		},
		"op !=; OK ; float": {
			Left:      workflow.ConstValue{Value: 24.13},
			Right:     workflow.ConstValue{Value: 42.13},
			Operation: "!=",
			IsOK:      true,
			HasError:  nil,
		},
		"op !=; not OK ; float": {
			Left:      workflow.ConstValue{Value: 42.13},
			Right:     workflow.ConstValue{Value: 42.13},
			Operation: "!=",
			IsOK:      false,
			HasError:  nil,
		},
		"op <; OK ; float": {
			Left:      workflow.ConstValue{Value: 24.13},
			Right:     workflow.ConstValue{Value: 42.13},
			Operation: "<",
			IsOK:      true,
			HasError:  nil,
		},
		"op <; not OK; float": {
			Left:      workflow.ConstValue{Value: 42.13},
			Right:     workflow.ConstValue{Value: 24.13},
			Operation: "<",
			IsOK:      false,
			HasError:  nil,
		},
		"op >; OK ; float": {
			Left:      workflow.ConstValue{Value: 42.13},
			Right:     workflow.ConstValue{Value: 24.13},
			Operation: ">",
			IsOK:      true,
			HasError:  nil,
		},
		"op >; not OK; float": {
			Left:      workflow.ConstValue{Value: 24.13},
			Right:     workflow.ConstValue{Value: 42.13},
			Operation: ">",
			IsOK:      false,
			HasError:  nil,
		},
		"op <=; OK - less; float": {
			Left:      workflow.ConstValue{Value: 24.13},
			Right:     workflow.ConstValue{Value: 42.13},
			Operation: "<=",
			IsOK:      true,
			HasError:  nil,
		},
		"op <=; OK - eq; float": {
			Left:      workflow.ConstValue{Value: 42.13},
			Right:     workflow.ConstValue{Value: 42.13},
			Operation: "<=",
			IsOK:      true,
			HasError:  nil,
		},
		"op <=; not OK; float": {
			Left:      workflow.ConstValue{Value: 42.13},
			Right:     workflow.ConstValue{Value: 24.13},
			Operation: "<=",
			IsOK:      false,
			HasError:  nil,
		},
		"op >=; OK - less; float": {
			Left:      workflow.ConstValue{Value: 42.13},
			Right:     workflow.ConstValue{Value: 24.13},
			Operation: ">=",
			IsOK:      true,
			HasError:  nil,
		},
		"op >=; OK - eq; float": {
			Left:      workflow.ConstValue{Value: 42.13},
			Right:     workflow.ConstValue{Value: 42.13},
			Operation: ">=",
			IsOK:      true,
			HasError:  nil,
		},
		"op >=; not OK; float": {
			Left:      workflow.ConstValue{Value: 24.13},
			Right:     workflow.ConstValue{Value: 42.13},
			Operation: ">=",
			IsOK:      false,
			HasError:  nil,
		},
		// float + int
		"op ==; OK ; int+float": {
			Left:      workflow.ConstValue{Value: 42.0},
			Right:     workflow.ConstValue{Value: 42},
			Operation: "==",
			IsOK:      true,
			HasError:  nil,
		},
		"op ==; not OK ; int+float": {
			Left:      workflow.ConstValue{Value: 42.0},
			Right:     workflow.ConstValue{Value: 24},
			Operation: "==",
			IsOK:      false,
			HasError:  nil,
		},
		"op !=; OK ; int+float": {
			Left:      workflow.ConstValue{Value: 24.0},
			Right:     workflow.ConstValue{Value: 42},
			Operation: "!=",
			IsOK:      true,
			HasError:  nil,
		},
		"op !=; not OK ; int+float": {
			Left:      workflow.ConstValue{Value: 42.0},
			Right:     workflow.ConstValue{Value: 42},
			Operation: "!=",
			IsOK:      false,
			HasError:  nil,
		},
		"op <; OK ; int+float": {
			Left:      workflow.ConstValue{Value: 24.0},
			Right:     workflow.ConstValue{Value: 42},
			Operation: "<",
			IsOK:      true,
			HasError:  nil,
		},
		"op <; not OK; int+float": {
			Left:      workflow.ConstValue{Value: 42.0},
			Right:     workflow.ConstValue{Value: 24},
			Operation: "<",
			IsOK:      false,
			HasError:  nil,
		},
		"op >; OK ; int+float": {
			Left:      workflow.ConstValue{Value: 42.0},
			Right:     workflow.ConstValue{Value: 24},
			Operation: ">",
			IsOK:      true,
			HasError:  nil,
		},
		"op >; not OK; int+float": {
			Left:      workflow.ConstValue{Value: 24.0},
			Right:     workflow.ConstValue{Value: 42},
			Operation: ">",
			IsOK:      false,
			HasError:  nil,
		},
		"op <=; OK - less; int+float": {
			Left:      workflow.ConstValue{Value: 24.0},
			Right:     workflow.ConstValue{Value: 42},
			Operation: "<=",
			IsOK:      true,
			HasError:  nil,
		},
		"op <=; OK - eq; int+float": {
			Left:      workflow.ConstValue{Value: 42.0},
			Right:     workflow.ConstValue{Value: 42},
			Operation: "<=",
			IsOK:      true,
			HasError:  nil,
		},
		"op <=; not OK; int+float": {
			Left:      workflow.ConstValue{Value: 42.0},
			Right:     workflow.ConstValue{Value: 24},
			Operation: "<=",
			IsOK:      false,
			HasError:  nil,
		},
		"op >=; OK - less; int+float": {
			Left:      workflow.ConstValue{Value: 42.0},
			Right:     workflow.ConstValue{Value: 24},
			Operation: ">=",
			IsOK:      true,
			HasError:  nil,
		},
		"op >=; OK - eq; int+float": {
			Left:      workflow.ConstValue{Value: 42.0},
			Right:     workflow.ConstValue{Value: 42},
			Operation: ">=",
			IsOK:      true,
			HasError:  nil,
		},
		"op >=; not OK; int+float": {
			Left:      workflow.ConstValue{Value: 24.0},
			Right:     workflow.ConstValue{Value: 42},
			Operation: ">=",
			IsOK:      false,
			HasError:  nil,
		},
		"op ==; not OK - float not a round number; int+float": {
			Left:      workflow.ConstValue{Value: 42},
			Right:     workflow.ConstValue{Value: 42.12},
			Operation: "==",
			IsOK:      false,
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
			t.Must.Equal(ok, tc.IsOK)
		}
	})
}

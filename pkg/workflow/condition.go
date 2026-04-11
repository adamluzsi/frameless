package workflow

import "context"

type ExecuteCondition struct {
	ID    ConditionID   `json:"id"`
	Input []VariableKey `json:"input,omitempty"`
}

func (ec ExecuteCondition) Evaluate(ctx context.Context, p *Process) (bool, error) {
	return false, nil
}

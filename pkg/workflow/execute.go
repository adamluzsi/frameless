package workflow

import (
	"context"
	"fmt"
)

// Execute is the unit of idempotent work in a workflow.
// It executes either a step, which is a Definition, or an answer, which is a Condition:
//
//   - Execute calls the participant registered under ParticipantID,
//     and ExecuteWith runs the given function in place of a registered participant.
//   - Evaluate asks the condition registered under ConditionID,
//     and EvaluateWith asks the given function in place of a registered condition.
//
// Execute and Evaluate need exactly the ID of their own role to be set.
// ExecuteWith and EvaluateWith take the ID as an argument instead, so neither ID field may be set.
// Any other combination fails with a fatal ErrInvalidDefinition.
//
// The ID together with the current workflow.Path identifies the execution.
// The first successful execution is recorded in the process history,
// and a later execution at the same position replays it instead of executing again,
// as long as the Input values are the same as they were at the recorded position.
// Errors and runtime signals (e.g. workflow.Suspend) are not recorded as an execution,
// so the next execution tries again.
//
// A step is recorded as an EventParticipant at the participant/<ParticipantID> path,
// and an answer as an EventCondition at the <ConditionID> path, relative to the current path.
type Execute struct {
	ParticipantID ParticipantID
	ConditionID   ConditionID
	// Input names the process variables whose values are passed to the participant or condition, in order.
	Input []VarName
	// Output names the process variables that store the results of a participant, in order.
	// A condition's answer is not stored in variables, so a condition can't have Output.
	Output []VarName
}

func (Execute) Error() string { return "workflow::execute" }

var _ Definition = Execute{}

// Execute calls the participant registered under ParticipantID with the Input values,
// and stores its results in the Output variables.
//
// A participant that isn't registered on this node, or can't take the Input and Output mapping,
// fails with a nonfatal error, so another node can execute the process.
// Other failures are recorded as an EventError.
// A workflow.Definition returned as the participant's error is recorded with the call,
// and runs in place of the step, on replay too.
func (d Execute) Execute(ctx context.Context, pid ProcessID) error {
	if err := d.checkStep(); err != nil {
		return err
	}
	return d.executeStep(ctx, pid, d.ParticipantID, d.callParticipant)
}

// ExecuteWith runs executeFn in place of a registered participant,
// with the replay semantics of Execute#Execute.
//
// The id together with the current workflow.Path identifies the call,
// so keep it stable, namespaced (e.g. "acme::charge-card"),
// and distinct from the other executions at the same position.
// Neither ParticipantID nor ConditionID may be set on the Execute.
// No participant has to be registered under the id,
// and a call recorded by ExecuteWith or by Execute with the participant registered under the same ID
// is replayed by the other,
// unless it was recorded with an Output mapping, which is part of the call's identity.
// The first successful call is recorded, and every later execution at the same position replays it
// without calling executeFn again.
// Errors and runtime signals (e.g. workflow.Suspend) are not recorded as a call,
// so the next execution calls executeFn again.
// A workflow.Definition returned as the error is recorded with the call and runs in place of the step,
// the same way as a participant's follow-up definition.
//
// Input is resolved and recorded along with the call the same way as with Execute,
// but executeFn is responsible for reading whatever it needs from the process.
// executeFn returns no values, so Output must be empty;
// the variables it sets through workflow.Vars are recorded as part of the call instead.
//
// Its signature matches Definition#Execute,
// so a Definition implementation can make its own side effects replay-safe by passing its own execution logic,
// without registering a participant for it.
func (d Execute) ExecuteWith(ctx context.Context, pid ProcessID, id ParticipantID, executeFn func(ctx context.Context, processID ProcessID) error) error {
	if executeFn == nil {
		return ErrFatal.F("%s: missing execute function", d.Error())
	}
	if err := d.checkWithID("ExecuteWith", string(id)); err != nil {
		return err
	}
	if len(d.Output) != 0 {
		return ErrInvalidDefinition.F("%s: %s maps %d Output variables, but an execute function returns no values; set them through workflow.Vars instead",
			d.Error(), id, len(d.Output))
	}
	return d.executeStep(ctx, pid, id, func(ctx context.Context, _ []any) ([]any, error) {
		return nil, executeFn(ctx, pid)
	})
}

var _ Condition = Execute{}

// Evaluate asks the condition registered under ConditionID with the Input values.
func (d Execute) Evaluate(ctx context.Context, pid ProcessID) (bool, error) {
	if err := d.checkCondition(); err != nil {
		return false, err
	}
	return d.evaluate(ctx, pid, d.ConditionID, d.callCondition)
}

// EvaluateWith answers with evaluateFn in place of a registered condition,
// with the replay semantics of Execute#Evaluate.
//
// The id together with the current workflow.Path identifies the answer,
// so keep it stable, namespaced (e.g. "acme::approval-granted"),
// and distinct from the other evaluations at the same position.
// Neither ParticipantID nor ConditionID may be set on the Execute.
// No condition has to be registered under the id,
// and an answer recorded by EvaluateWith or by Evaluate with the condition registered under the same ID
// is replayed by the other.
// The first successful answer is recorded, and every later evaluation at the same position replays it
// without calling evaluateFn again.
// Errors and runtime signals (e.g. workflow.Suspend) are not answers,
// so they are not recorded, and the next evaluation calls evaluateFn again.
//
// Input is resolved and recorded along with the answer the same way as with Evaluate,
// but evaluateFn is responsible for reading whatever it needs from the process.
// A condition's answer is not stored in variables, so Output must be empty.
//
// Its signature matches Condition#Evaluate,
// so a Condition implementation can make its own answers replay-stable by passing its own evaluation logic.
func (d Execute) EvaluateWith(ctx context.Context, pid ProcessID, id ConditionID, evaluateFn func(ctx context.Context, processID ProcessID) (bool, error)) (bool, error) {
	if evaluateFn == nil {
		return false, ErrFatal.F("%s: missing evaluate function", d.Error())
	}
	if err := d.checkWithID("EvaluateWith", string(id)); err != nil {
		return false, err
	}
	if len(d.Output) != 0 {
		return false, ErrInvalidDefinition.F("%s: %s maps %d Output variables, but a condition's answer is not stored in variables",
			d.Error(), id, len(d.Output))
	}
	return d.evaluate(ctx, pid, id, func(ctx context.Context, _ []any) ([]any, error) {
		ok, err := evaluateFn(ctx, pid)
		return []any{ok}, err
	})
}

func (d Execute) checkStep() error {
	switch {
	case d.ParticipantID != "" && d.ConditionID != "":
		return d.errBothIDs()
	case d.ParticipantID != "":
		return nil
	case d.ConditionID != "":
		return ErrInvalidDefinition.F("%s: condition %s answers with a bool, so it can't be executed as a step; use it as a workflow.Condition (e.g. If#Cond), or set ParticipantID instead",
			d.Error(), d.ConditionID)
	default:
		return ErrInvalidDefinition.F("%s: missing ParticipantID", d.Error())
	}
}

func (d Execute) checkCondition() error {
	switch {
	case d.ParticipantID != "" && d.ConditionID != "":
		return d.errBothIDs()
	case d.ParticipantID != "":
		return ErrInvalidDefinition.F("%s: participant %s is executed as a step, so it can't answer as a workflow.Condition; set ConditionID instead",
			d.Error(), d.ParticipantID)
	case d.ConditionID == "":
		return ErrInvalidDefinition.F("%s: missing ConditionID", d.Error())
	case len(d.Output) != 0:
		return ErrInvalidDefinition.F("%s: condition %s maps %d Output variables, but a condition's answer is not stored in variables",
			d.Error(), d.ConditionID, len(d.Output))
	}
	return nil
}

func (d Execute) errBothIDs() error {
	return ErrInvalidDefinition.F("%s: both ParticipantID (%s) and ConditionID (%s) are set, but only one of them can be executed",
		d.Error(), d.ParticipantID, d.ConditionID)
}

// checkWithID validates an Execute for ExecuteWith and EvaluateWith,
// which take the ID that identifies the execution as an argument instead of an ID field.
func (d Execute) checkWithID(method, id string) error {
	if id == "" {
		return ErrFatal.F("%s: %s is missing the ID that identifies the execution", d.Error(), method)
	}
	if d.ParticipantID != "" || d.ConditionID != "" {
		return ErrInvalidDefinition.F("%s: %s identifies the execution by its id argument (%s), but ParticipantID (%s) or ConditionID (%s) is set; remove them, or call Execute/Evaluate to use the registered participant or condition",
			d.Error(), method, id, d.ParticipantID, d.ConditionID)
	}
	return nil
}

///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

func (d Execute) executeStep(ctx context.Context, pid ProcessID, id ParticipantID, do nonIdempotentFunc) error {
	ctx = WithName(ctx, "participant")
	exec := idempotentExecutor[EventParticipant, ParticipantID]{
		ID:     id,
		Do:     do,
		Input:  d.Input,
		Output: d.Output,
		CastEvent: func(e EventParticipant) (executionEvent[ParticipantID], bool) {
			return executionEvent[ParticipantID]{
				ID:         e.ParticipantID,
				Path:       e.Path,
				Input:      e.Input,
				Output:     e.Output,
				Definition: e.Definition,
			}, true
		},
		MakeEvent: func(id ParticipantID, path Path, input, output []any) (EventParticipant, error) {
			eventID, err := MakeEventID()
			if err != nil {
				return EventParticipant{}, err
			}
			return EventParticipant{
				EventID:       eventID,
				ProcessID:     pid,
				Timestamp:     timeNow(),
				ParticipantID: id,
				Path:          path,
				Input:         input,
				Output:        output,
			}, nil
		},
		AcceptDefinition: func(e *EventParticipant, def Definition) {
			e.Definition = def
		},
		MakeEventError: func(id ParticipantID, path Path, err error) (EventError, error) {
			eventID, mErr := MakeEventID()
			if mErr != nil {
				return EventError{}, mErr
			}
			return EventError{
				EventID:       eventID,
				ProcessID:     pid,
				Timestamp:     timeNow(),
				Path:          path,
				Error:         err.Error(),
				ParticipantID: id,
			}, nil
		},
	}
	return exec.Execute(ctx, pid)
}

func (d Execute) callParticipant(ctx context.Context, input []any) (_output []any, _ error) {
	pr, ok := ctxParticipantsH.Lookup(ctx)
	if !ok {
		return nil, ErrParticipantNotFound{ID: d.ParticipantID}
	}
	participant, found, err := pr.FindByID(ctx, d.ParticipantID)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, ErrParticipantNotFound{ID: d.ParticipantID}
	}

	fn, err := participant.rFunc()
	if err != nil {
		return nil, ErrParticipantSignatureMismatch{ID: d.ParticipantID, Cause: err}
	}

	args, err := invocationArgs(fn.Type(), ctx, input)
	if err != nil {
		return nil, ErrParticipantSignatureMismatch{ID: d.ParticipantID, Cause: ErrParticipantFuncMappingMismatch.F(
			"%v\nsignature: %s", err, participant.funcSignature())}
	}

	// rFunc validates the trailing error; it has no output variable mapping.
	var expectedOutputMappingLen = fn.Type().NumOut() - 1
	if len(d.Output) != expectedOutputMappingLen {
		return nil, ErrParticipantSignatureMismatch{ID: d.ParticipantID, Cause: ErrParticipantFuncMappingMismatch.F(
			"output count mismatch: got %d, want %d\nsignature: %s", len(d.Output), expectedOutputMappingLen, participant.funcSignature())}
	}

	var out = invoke(fn, args)
	if err, ok := out[len(out)-1].Interface().(error); ok && err != nil {
		return nil, err
	}
	out = out[:len(out)-1]

	var output []any

	for _, val := range out {
		output = append(output, val.Interface())
	}

	return output, nil
}

///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

func (d Execute) evaluate(ctx context.Context, pid ProcessID, id ConditionID, do nonIdempotentFunc) (bool, error) {
	exec := idempotentExecutor[EventCondition, ConditionID]{
		ID: id,
		Do: func(ctx context.Context, input []any) ([]any, error) {
			output, err := do(ctx, input)
			// The idempotent executor treats a Definition error as a follow-up to run,
			// which is meaningful for a step, but a condition can only answer with a bool.
			if def, ok := err.(Definition); ok {
				return nil, ErrFatal.F("%s: condition %s returned a %T definition instead of an answer", d.Error(), id, def)
			}
			return output, err
		},
		Input: d.Input,
		CastEvent: func(e EventCondition) (executionEvent[ConditionID], bool) {
			return executionEvent[ConditionID]{
				ID:     e.ConditionID,
				Path:   e.Path,
				Input:  e.Input,
				Result: []any{e.Answer},
			}, true
		},
		MakeEvent: func(id ConditionID, path Path, input, output []any) (EventCondition, error) {
			eventID, err := MakeEventID()
			if err != nil {
				return EventCondition{}, err
			}
			return EventCondition{
				EventID:     eventID,
				ProcessID:   pid,
				ConditionID: id,
				Path:        path,
				Input:       input,
				Answer:      output[0].(bool),
				Timestamp:   timeNow(),
			}, nil
		},
	}

	outs, err := exec.executeWR(ctx, pid)
	if err != nil {
		return false, err
	}
	if len(outs) != 1 {
		return false, fmt.Errorf("incorrect condition caching implementation, expected 1 boolean result, but got %d", len(outs))
	}
	if answer, ok := outs[0].(bool); ok {
		return answer, nil
	}
	return false, fmt.Errorf("incorrect condition caching implementation, expected 1 boolean result, but got type %T", outs[0])
}

func (d Execute) callCondition(ctx context.Context, input []any) ([]any, error) {
	pr, ok := ctxConditionsH.Lookup(ctx)
	if !ok {
		return nil, ErrFatal.F("missing condition mapping from workflow runtime")
	}
	condition, found, err := pr.FindByID(ctx, d.ConditionID)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, ErrConditionNotFound{ID: d.ConditionID}
	}
	cw, ok := condition.(*conditionWrapper)
	if !ok || cw == nil {
		return nil, ErrInvalidConditionFunc.F("condition %s must be a registered condition function, got %T", d.ConditionID, condition)
	}
	answer, err := cw.evaluate(ctx, input)
	if err != nil {
		return nil, err
	}
	return []any{answer}, nil
}

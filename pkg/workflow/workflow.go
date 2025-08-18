package workflow

import (
	"context"
	"encoding/json"
	"fmt"

	"go.llib.dev/frameless/pkg/mapkit"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/pkg/synckit"
	"go.llib.dev/frameless/pkg/validate"
)

type ErrParticipantNotFound struct {
	PID ParticipantID
}

func (e ErrParticipantNotFound) Error() string {
	return fmt.Sprintf("[ErrParticipantNotFound] %s", e.PID)
}

type JSONSerialisable interface {
	json.Marshaler
	json.Unmarshaler
}

type Condition interface {
	minCondition
	minDefinition
}

type minCondition interface {
	Evaluate(ctx context.Context, s *State) (bool, error)
}

type Process struct {
	PDEF  Definition `json:"pdef"`
	State *State     `json:"state"`
}

type State struct {
	Variables synckit.Map[string, any]
}

func (s *State) Merge(oth *State) {
	if s == nil {
		panic(fmt.Sprintf("nil %T", s))
	}
	if oth == nil {
		return
	}
	vs := mapkit.Merge(s.Variables.ToMap(), oth.Variables.ToMap())
	for k, v := range vs {
		s.Variables.Set(k, v)
	}
}

type Variables map[string]any

func (vs Variables) Validate(context.Context) error {
	data, err := json.Marshal(vs)
	if err != nil {
		return fmt.Errorf("workflow variables are not valid because the values must be json encodable")
	}
	var got Variables
	if err := json.Unmarshal(data, &vs); err != nil {
		return fmt.Errorf("workflow variables are not valid because the json encoded values should be okay to unmarshal")
	}
	if !reflectkit.Equal(vs, got) {
		return fmt.Errorf("workflow variables are not valid because json encoding should not affect its values")
	}
	return nil
}

type Participants map[ParticipantID]Participant

type Participant interface {
	Execute(ctx context.Context, s *State) error
}

type ParticipantFunc func(ctx context.Context, s *State) error

var _ Participant = (*ParticipantFunc)(nil)

func (fn ParticipantFunc) Execute(ctx context.Context, s *State) error {
	return fn(ctx, s)
}

func vd(ctx context.Context, name string, v validate.Validatable, req bool) error {
	if req && v == nil {
		return validate.Error{Cause: fmt.Errorf("%s is missing", name)}
	}
	if v == nil {
		return nil
	}
	return v.Validate(ctx)
}

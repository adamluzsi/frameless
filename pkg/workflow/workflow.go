package workflow

import (
	"context"
	"encoding/json"
	"fmt"

	"go.llib.dev/frameless/pkg/mapkit"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/pkg/validate"
	"go.llib.dev/frameless/port/ds"
)

type JSONSerialisable interface {
	json.Marshaler
	json.Unmarshaler
}

type minCondition interface {
	Evaluate(ctx context.Context, s *State) (bool, error)
}

type Process struct {
	Definition Definition `json:"pdef"`
	State      *State     `json:"state"`
}

type Cache struct {
}

type State struct {
	Variables Variables
}

func (s *State) Merge(oth *State) {
	if s == nil {
		panic(fmt.Sprintf("nil %T", s))
	}
	if oth == nil {
		return
	}
	for k, v := range mapkit.Merge(s.Variables.Map, oth.Variables.Map) {
		s.Variables.Map.Set(k, v)
	}
}

type Variables struct {
	ds.Map[VariableKey, any]
}

type VariableKey string

var _ ds.Map[VariableKey, any] = (*Variables)(nil)

func (vs Variables) MarshalJSON() ([]byte, error) {
	return json.Marshal(vs.Map)
}

func (vs *Variables) UnmarshalJSON(data []byte) error {
	return json.Unmarshal(data, &vs.Map)
}

func (vs Variables) Validate(context.Context) error {
	data, err := json.Marshal(vs)
	if err != nil {
		return fmt.Errorf("workflow variables are not valid because the values must be json encodable")
	}
	var got Variables
	if err := json.Unmarshal(data, &got); err != nil {
		return fmt.Errorf("workflow variables are not valid because the json encoded values should be okay to unmarshal")
	}
	if !reflectkit.Equal(vs, got) {
		return fmt.Errorf("workflow variables are not valid because json encoding should not affect its values")
	}
	return nil
}

type ParticipantMapping map[ParticipantID]Participant

var _ ParticipantRepository = (ParticipantMapping)(nil)

func (ps ParticipantMapping) FindByID(ctx context.Context, id ParticipantID) (Participant, bool, error) {
	if len(ps) == 0 {
		var zero Participant
		return zero, false, nil
	}
	p, ok := ps[id]
	return p, ok, nil
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

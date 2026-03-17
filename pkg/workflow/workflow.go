package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"go.llib.dev/frameless/pkg/mapkit"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/pkg/validate"
	"go.llib.dev/frameless/port/ds"
	"go.llib.dev/frameless/port/ds/dsmap"
)

type Definition interface {
	minDefinition
	Execute(ctx context.Context, r Runtime, p *Process) error
}

var _ minDefinition = (Definition)(nil)

type minDefinition interface {
	JSONSerialisable
	// ValidateDefinition(ctx ValidateDefinitionContext) error
	// validate.Validatable
}

type Condition interface {
	minCondition
	minDefinition
}

var _ minCondition = (Condition)(nil)

type minCondition interface {
	Evaluate(ctx context.Context, p *Process) (bool, error)
}

type Process struct {
	Definition Definition `json:"pdef"`
	Variables  Variables  `json:"var"`

	m sync.Mutex
}

// TODO: rename to Process
type State struct {
	vs dsmap.Map[VariableKey, any]
}

func (s *State) Validate(ctx context.Context) error {
	return s.validateVariables(ctx)
}

func (s *State) validateVariables(ctx context.Context) error {
	data, err := json.Marshal(s.vs)
	if err != nil {
		return fmt.Errorf("workflow variables are not valid because the values must be json encodable")
	}
	var got dsmap.Map[VariableKey, any]
	if err := json.Unmarshal(data, &got); err != nil {
		return fmt.Errorf("workflow variables are not valid because the json encoded values should be okay to unmarshal")
	}
	if !reflectkit.Equal(s.vs, got) {
		return fmt.Errorf("workflow variables are not valid because json encoding should not affect its values")
	}
	return nil
}

func (s *State) Merge(oth *State) {
	if s == nil {
		panic(fmt.Sprintf("nil %T", s))
	}
	if oth == nil {
		return
	}
	for k, v := range mapkit.Merge(s.vs, oth.vs) {
		s.vs.Set(k, v)
	}
}

type Variables struct {
	vs ds.Map[VariableKey, any]
}

type VariableKey string

type ParticipantMapping map[ParticipantID]any

var _ ParticipantRepository = (ParticipantMapping)(nil)

func (ps ParticipantMapping) FindByID(ctx context.Context, id ParticipantID) (Participant, bool, error) {
	if len(ps) == 0 {
		var zero Participant
		return zero, false, nil
	}
	fn, ok := ps[id]
	return Participant{ID: id, Func: fn}, ok, nil
}

func (ps ParticipantMapping) Validate(ctx context.Context) error {
	for id, fn := range ps {
		p := Participant{
			ID:   id,
			Func: fn,
		}
		if err := p.Validate(ctx); err != nil {
			return fmt.Errorf("%s: %w", id, err)
		}
	}
	return nil
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

type JSONSerialisable interface {
	json.Marshaler
	json.Unmarshaler
}

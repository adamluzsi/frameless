package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/port/comproto"
	"go.llib.dev/frameless/port/pubsub"
)

const ErrParticipantNotFound errorkit.Error = "ErrParticipantNotFound"

type JSONSerialisable interface {
	json.Marshaler
	json.Unmarshaler
}

type Process struct {
	PDEF  ProcessDefinition `json:"pdef"`
	State *State            `json:"state"`
}

var _ JSONSerialisable = (*Process)(nil)

func (p Process) MarshalJSON() ([]byte, error) {
	type DTO Process
	return json.Marshal(DTO(p))
}

func (p *Process) UnmarshalJSON(data []byte) error {
	type DTO Process
	var dto DTO
	if err := json.Unmarshal(data, &dto); err != nil {
		return err
	}
	*p = Process(dto)
	return nil
}

type ProcessQueue interface {
	pubsub.Publisher[Process]
	pubsub.Subscriber[Process]
}

type Runtime struct {
	Queue ProcessQueue

	Participants    Participants
	TemplateFuncMap TemplateFuncMap
}

func (r Runtime) Run(ctx context.Context) error {
	for msg, err := range r.Queue.Subscribe(ctx) {
		if err != nil {
			return err
		}

		r.Execute(msg.Context(), msg.Data())

		comproto.FinishTx(&err, msg.ACK, msg.NACK)
	}
	return nil
}

func (r Runtime) Context(ctx context.Context) context.Context {
	ctx = ContextWithParticipants(ctx, r.Participants)
	ctx = ContextWithFuncMap(ctx, r.TemplateFuncMap)
	return ctx
}

func (r Runtime) Execute(ctx context.Context, p Process) error {
	ctx = r.Context(ctx)
	if err := p.PDEF.Validate(ctx); err != nil {
		return err
	}

	if p.State == nil {
		p.State = NewState()
	}

	err := p.PDEF.Execute(ctx, p.State)

	// handle different control error statement
	return err
}

func NewState() *State {
	return &State{
		Variables: make(Variables),
	}
}

type State struct {
	Variables Variables `json:"vars"`
}

var _ JSONSerialisable = (*State)(nil)

func (s State) MarshalJSON() ([]byte, error) {
	type DTO State
	return json.Marshal(DTO(s))
}

func (s *State) UnmarshalJSON(data []byte) error {
	type DTO State
	var dto DTO
	if err := json.Unmarshal(data, &dto); err != nil {
		return err
	}
	*s = State(dto)
	return nil
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
	Execute(context.Context, *State) error
}

type ParticipantFunc func(ctx context.Context, s *State) error

var _ Participant = (*ParticipantFunc)(nil)

func (fn ParticipantFunc) Execute(ctx context.Context, s *State) error {
	return fn(ctx, s)
}

type TemplateFuncMap map[string]any

func (fm TemplateFuncMap) Validate(context.Context) error {
	for name, fn := range fm {
		fnType := reflect.TypeOf(fn)

		if fnType.Kind() != reflect.Func {
			const format = "invalid workflow.FuncMap value for %s, expected function but got %s"
			return fmt.Errorf(format, name, fnType.Kind().String())
		}
	}
	return nil
}

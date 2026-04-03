package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"reflect"
	"strings"

	"go.llib.dev/frameless/internal/errorkitlite"
	"go.llib.dev/frameless/pkg/jsonkit"
	"go.llib.dev/frameless/pkg/mapkit"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/pkg/validate"
	"go.llib.dev/frameless/port/codec"
	"go.llib.dev/frameless/port/crud"
	"go.llib.dev/frameless/port/ds"
	"go.llib.dev/frameless/port/ds/dsmap"
)

type Definition interface {
	Execute(ctx context.Context, p *Process) error
}

type Condition interface {
	Evaluate(ctx context.Context, p *Process) (bool, error)
}

type ConditionID string

type ConditionConveratble interface {
	ToCondition(ctx context.Context, p *Process) (Condition, bool)
}

///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

// Runtime is the default runtime to execute process definitons.
// It can be extended or reimplemented if it doesn't fit your workflow realted use-cases.
type Runtime struct {
	Participants ParticipantRepository
	Conditions   ConditionRepository
	Codec        codec.Codec
}

type ParticipantRepository interface {
	crud.ByIDFinder[Participant, ParticipantID]
}

type ConditionRepository interface {
	crud.ByIDFinder[Condition, ConditionID]
}

func (r Runtime) Context(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if r.Participants != nil {
		ctx = ContextWithParticipants(ctx, r.Participants)
	}
	if r.Conditions != nil {
		ctx = ContextWithConditions(ctx, r.Conditions)
	}
	// add participant execution cache too cache
	return ctx
}

///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

type Process struct {
	Definition   Definition     `json:"pdef"`
	Variables    Variables      `json:"var"`
	ExecuteEvent []ExecuteEvent `json:"execute-events"`
}

///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

type Variables struct {
	vs dsmap.Map[VariableKey, any]
}

type VariableKey string

var _ ds.ReadOnlyMap[VariableKey, any] = Variables{}
var _ ds.Map[VariableKey, any] = (*Variables)(nil)
var _ ds.MapConveratble[VariableKey, any] = (*Variables)(nil)

func (vs Variables) Lookup(key VariableKey) (any, bool) { return vs.vs.Lookup(key) }
func (vs Variables) Get(key VariableKey) any            { return vs.vs.Get(key) }
func (vs Variables) All() iter.Seq2[VariableKey, any]   { return vs.vs.All() }
func (cs *Variables) Set(key VariableKey, val any)      { cs.vs.Set(key, val) }
func (cs *Variables) Delete(key VariableKey)            { cs.vs.Delete(key) }
func (cs *Variables) ToMap() map[VariableKey]any        { return mapkit.Clone(cs.ToMap()) }

func (vs Variables) Validate(ctx context.Context) error {
	return vs.validateVariables(ctx)
}

func (vs *Variables) Merge(oth Variables) {
	if oth.vs == nil {
		return
	}
	for k, v := range oth.vs.All() {
		vs.vs.Set(k, v)
	}
}

func (s *Variables) validateVariables(context.Context) error {
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

///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

// Participant is a logical unit implemented at workflow engine-level.
//
// If ParticipantRepository is supplied to the workflow runtime context,
// then registered particpants can be used from within workflow definitions.
type Participant struct {
	ID   ParticipantID
	Func any // func(context.Context, ...) (..., error)
}

// funcSignature
//
// TODO: repalce with OpenAPI definition
func (p Participant) funcSignature(ctx context.Context) string {
	var fn, err = p.rfn(ctx)
	if err != nil {
		return ""
	}
	var (
		fnType = fn.Type()
		input  []string
		output []string
	)
	for i := range fnType.NumIn() {
		in := fnType.In(i)
		val := in.String()
		if in.IsVariadic() {
			val = "..." + val
		}
		input = append(input, in.String())
	}
	for i := range fnType.NumOut() {
		output = append(output, fnType.Out(i).String())
	}
	return fmt.Sprintf("func(%s) (%s)", strings.Join(input, ", "), strings.Join(output, ", "))
}

type ParticipantID string

var _ validate.Validatable = (*Participant)(nil)

const ErrInvalidParicipantFunc errorkitlite.Error = `Invalid workflow.Participant#Func signature:
expected func(context.Context, arg1 T1, ...OtherArgs) (Result1, Result2..., error)
where the function signature starts with a context.Context, then user defined argument types,
and the results tuple is also returns user defined types, ending with an error value type.
The input and output argument types must be serializable.
`

var reflectContextType = reflectkit.TypeOf[context.Context]()

var reflectErrorType = reflectkit.TypeOf[error]()

func (p Participant) rfn(ctx context.Context) (reflect.Value, error) {
	rfunc := reflect.ValueOf(p.Func)
	if rfunc.Kind() != reflect.Func {
		return rfunc, ErrInvalidParicipantFunc.F("invalid value for participant func")
	}
	var (
		funcType   = rfunc.Type()
		funcNumIn  = funcType.NumIn()
		funcNumOut = funcType.NumOut()
	)
	if funcNumIn < 1 {
		return rfunc, ErrInvalidParicipantFunc
	}
	if funcType.In(0) != reflectContextType {
		return rfunc, ErrInvalidParicipantFunc
	}
	if funcNumOut < 1 {
		return rfunc, ErrInvalidParicipantFunc
	}
	if lastOut := funcType.Out(funcNumOut - 1); lastOut != reflectErrorType || !lastOut.Implements(reflectErrorType) {
		return rfunc, ErrInvalidParicipantFunc
	}
	return rfunc, nil
}

func (p Participant) Validate(ctx context.Context) error {
	_, err := p.rfn(ctx)
	return err
}

///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

type Participants map[ParticipantID]any

var _ ParticipantRepository = (Participants)(nil)

func (ps Participants) FindByID(ctx context.Context, id ParticipantID) (Participant, bool, error) {
	if len(ps) == 0 {
		var zero Participant
		return zero, false, nil
	}
	fn, ok := ps[id]
	return Participant{ID: id, Func: fn}, ok, nil
}

func (ps Participants) Validate(ctx context.Context) error {
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

///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

func DefaultJSONCodec() jsonkit.Codec {
	var c jsonkit.Codec
	return c
}

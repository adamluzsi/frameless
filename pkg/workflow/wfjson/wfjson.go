package wfjson

import (
	"encoding/json"
	"time"

	"go.llib.dev/frameless/pkg/jsonkit"
	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/frameless/pkg/workflow/wftemplate"
)

// NewCodec returns a fresh workflow.Codec for (de)serialising workflow values.
// Each call returns a fresh *jsonkit.Codec whose per-codec registry carries
// the workflow-specific TypeID aliases and per-type custom codecs. The TypeID
// values are kept on this codec so it can resolve a Definition / Condition
// concrete type by its $type tag without depending on the global registry
// being initialised.
//
// Workflow.Runtimes fall back to this when their Codec field is nil, so users
// can encode/decode any workflow value without additional setup.
func NewCodec() *jsonkit.Codec {
	var c jsonkit.Codec

	// The type IDs are namespaced, in the form of prefix:name, like workflow:sequence,
	// where the workflow prefix keeps them apart from the type IDs of other packages.
	// Each type ID was spelt with a double colon before, like workflow::sequence.
	// The former type IDs are linked to the current ones as aliases at the end,
	// so the event logs and definitions written with them are still read.

	// Conditions: wftemplate.Condition is a named primitive (string)
	// whose $type envelope is handled by jsonkit's reflect-based path,
	// so CodecRegisterTypeID is enough. workflow.Execute has a custom
	// codec (DTO shape diverges from the workflow struct shape).
	jsonkit.CodecRegisterTypeID[wftemplate.Condition](&c, "workflow:template:condition")

	// Definitions: every concrete type gets a custom codec because the
	// wire format (snake_case, DTO shape, nested $type envelopes) is
	// owned by this package and may diverge from the workflow struct
	// shape over time.
	jsonkit.CodecRegister[workflow.Sequence](&c, "workflow:sequence", WorkflowSequence{})
	jsonkit.CodecRegister[workflow.If](&c, "workflow:if", WorkflowIf{})
	jsonkit.CodecRegister[workflow.Sleep](&c, "workflow:sleep", WorkflowSleep{})
	jsonkit.CodecRegister[workflow.For](&c, "workflow:for", WorkflowFor{})
	jsonkit.CodecRegister[workflow.ForEach](&c, "workflow:foreach", WorkflowForEach{})
	// Break carries no state, so the reflect-based path is enough: it needs a
	// wire identity, not a shape.
	jsonkit.CodecRegisterTypeID[workflow.Break](&c, "workflow:break")
	jsonkit.CodecRegister[workflow.SetVar](&c, "workflow:var:set", WorkflowSetVar{})
	jsonkit.CodecRegister[workflow.DeclareVar](&c, "workflow:var:declare", WorkflowDeclareVar{})
	jsonkit.CodecRegister[workflow.DeleteVar](&c, "workflow:var:delete", WorkflowDeleteVar{})
	jsonkit.CodecRegister[workflow.Increment](&c, "workflow:op:increment", WorkflowIncrement{})
	jsonkit.CodecRegister[workflow.Spawn](&c, "workflow:spawn", WorkflowSpawn{})
	jsonkit.CodecRegister[workflow.Defer](&c, "workflow:defer", WorkflowDefer{})
	jsonkit.CodecRegister[workflow.Execute](&c, "workflow:execute", WorkflowExecute{})

	jsonkit.CodecRegister[workflow.Join](&c, "workflow:join", WorkflowJoin{})

	// Interfaces: no value is of an interface type, so these name no value,
	// but the element type of a slice or a map held as `any`, like a []workflow.Definition,
	// which is written as {"$type":"[]workflow:definition","$value":[...]}, so it is read back as the same type.
	jsonkit.CodecRegisterTypeID[workflow.Definition](&c, "workflow:definition")
	jsonkit.CodecRegisterTypeID[workflow.Condition](&c, "workflow:condition")

	// Events: same treatment — custom codecs own the wire format.
	jsonkit.CodecRegister[workflow.EventCompleted](&c, "workflow:event:completed", WorkflowEventCompleted{})
	jsonkit.CodecRegister[workflow.EventTerminated](&c, "workflow:event:terminated", WorkflowEventTerminated{})
	jsonkit.CodecRegister[workflow.EventDeclareVar](&c, "workflow:event:var:declare", WorkflowEventDeclareVar{})
	jsonkit.CodecRegister[workflow.EventSetVar](&c, "workflow:event:var:set", WorkflowEventSetVar{})
	jsonkit.CodecRegister[workflow.EventDeleteVar](&c, "workflow:event:var:delete", WorkflowEventDeleteVar{})
	jsonkit.CodecRegister[workflow.EventParticipant](&c, "workflow:event:participant", WorkflowEventParticipant{})
	jsonkit.CodecRegister[workflow.EventCondition](&c, "workflow:event:condition", WorkflowEventCondition{})
	jsonkit.CodecRegister[workflow.EventUseDefinition](&c, "workflow:event:use-definition", WorkflowEventUseDefinition{})
	jsonkit.CodecRegister[workflow.EventSpawn](&c, "workflow:event:spawn", WorkflowEventSpawn{})
	jsonkit.CodecRegister[workflow.EventJoin](&c, "workflow:event:join", WorkflowEventJoin{})
	jsonkit.CodecRegister[workflow.EventSleepCompleted](&c, "workflow:event:sleep:completed", WorkflowEventSleepCompleted{})
	jsonkit.CodecRegister[workflow.EventDefer](&c, "workflow:event:defer", WorkflowEventDefer{})
	jsonkit.CodecRegister[workflow.EventForEachIteration](&c, "workflow:event:foreach:iteration", WorkflowEventForEachIteration{})
	jsonkit.CodecRegister[workflow.EventError](&c, "workflow:event:error", WorkflowEventError{})

	// Schedule-side types: not part of Definition/Condition/Event but
	// still persisted across the runtime, so they need a stable wire format too.
	jsonkit.CodecRegister[workflow.ExecutionRequest](&c, "workflow:execution", WorkflowExecutionRequest{})
	jsonkit.CodecRegister[workflow.ProcessSchedule](&c, "workflow:schedule", WorkflowProcessSchedule{})
	jsonkit.CodecRegister[workflow.ProcessCancel](&c, "workflow:cancel", WorkflowEventProcessCancel{})

	// Former type IDs: each type ID was spelt with a double colon before, like workflow::sequence.
	// They are linked to the current type IDs as aliases, so the data written with them is read back as the same type,
	// while the values are written with the current type IDs.
	jsonkit.CodecRegisterTypeID[wftemplate.Condition](&c, "workflow:template:condition", "workflow::template::condition")
	jsonkit.CodecRegisterTypeID[workflow.Sequence](&c, "workflow:sequence", "workflow::sequence")
	jsonkit.CodecRegisterTypeID[workflow.If](&c, "workflow:if", "workflow::if")
	jsonkit.CodecRegisterTypeID[workflow.Sleep](&c, "workflow:sleep", "workflow::sleep")
	jsonkit.CodecRegisterTypeID[workflow.For](&c, "workflow:for", "workflow::for")
	jsonkit.CodecRegisterTypeID[workflow.ForEach](&c, "workflow:foreach", "workflow::foreach")
	jsonkit.CodecRegisterTypeID[workflow.Break](&c, "workflow:break", "workflow::break")
	jsonkit.CodecRegisterTypeID[workflow.SetVar](&c, "workflow:var:set", "workflow::var::set")
	jsonkit.CodecRegisterTypeID[workflow.DeclareVar](&c, "workflow:var:declare", "workflow::var::declare")
	jsonkit.CodecRegisterTypeID[workflow.DeleteVar](&c, "workflow:var:delete", "workflow::var::delete")
	jsonkit.CodecRegisterTypeID[workflow.Increment](&c, "workflow:op:increment", "workflow::op::increment")
	jsonkit.CodecRegisterTypeID[workflow.Spawn](&c, "workflow:spawn", "workflow::spawn")
	jsonkit.CodecRegisterTypeID[workflow.Defer](&c, "workflow:defer", "workflow::defer")
	jsonkit.CodecRegisterTypeID[workflow.Execute](&c, "workflow:execute", "workflow::execute")
	jsonkit.CodecRegisterTypeID[workflow.Join](&c, "workflow:join", "workflow::join")
	jsonkit.CodecRegisterTypeID[workflow.Definition](&c, "workflow:definition", "workflow::definition")
	jsonkit.CodecRegisterTypeID[workflow.Condition](&c, "workflow:condition", "workflow::condition")
	jsonkit.CodecRegisterTypeID[workflow.EventCompleted](&c, "workflow:event:completed", "workflow::event::completed")
	jsonkit.CodecRegisterTypeID[workflow.EventTerminated](&c, "workflow:event:terminated", "workflow::event::terminated")
	jsonkit.CodecRegisterTypeID[workflow.EventDeclareVar](&c, "workflow:event:var:declare", "workflow::event::var::declare")
	jsonkit.CodecRegisterTypeID[workflow.EventSetVar](&c, "workflow:event:var:set", "workflow::event::var::set")
	jsonkit.CodecRegisterTypeID[workflow.EventDeleteVar](&c, "workflow:event:var:delete", "workflow::event::var::delete")
	jsonkit.CodecRegisterTypeID[workflow.EventParticipant](&c, "workflow:event:participant", "workflow::event::participant")
	jsonkit.CodecRegisterTypeID[workflow.EventCondition](&c, "workflow:event:condition", "workflow::event::condition")
	jsonkit.CodecRegisterTypeID[workflow.EventUseDefinition](&c, "workflow:event:use-definition", "workflow::event::use-definition")
	jsonkit.CodecRegisterTypeID[workflow.EventSpawn](&c, "workflow:event:spawn", "workflow::event::spawn")
	jsonkit.CodecRegisterTypeID[workflow.EventJoin](&c, "workflow:event:join", "workflow::event::join")
	jsonkit.CodecRegisterTypeID[workflow.EventSleepCompleted](&c, "workflow:event:sleep:completed", "workflow::event::sleep::completed")
	jsonkit.CodecRegisterTypeID[workflow.EventDefer](&c, "workflow:event:defer", "workflow::event::defer")
	jsonkit.CodecRegisterTypeID[workflow.EventForEachIteration](&c, "workflow:event:foreach:iteration", "workflow::event::foreach::iteration")
	jsonkit.CodecRegisterTypeID[workflow.EventError](&c, "workflow:event:error", "workflow::event::error")
	jsonkit.CodecRegisterTypeID[workflow.ExecutionRequest](&c, "workflow:execution", "workflow::execution")
	jsonkit.CodecRegisterTypeID[workflow.ProcessSchedule](&c, "workflow:schedule", "workflow::schedule")
	jsonkit.CodecRegisterTypeID[workflow.ProcessCancel](&c, "workflow:cancel", "workflow::cancel")
	return &c
}

// WorkflowJoin is the codec for workflow.Join. It is kept here (rather than in
// dto.go) so its declaration stays next to the original example that anchored
// the per-type codec pattern.
type WorkflowJoin struct{}

type workflowJoinDTO struct {
	Spawn   string              `json:"spawn,omitzero"`
	Collect workflow.VarMapping `json:"collect,omitzero"`
}

func (WorkflowJoin) Marshal(c *jsonkit.Codec, v workflow.Join) ([]byte, error) {
	return json.Marshal(workflowJoinDTO{
		Spawn:   string(v.Spawn),
		Collect: v.Collect,
	})
}

func (WorkflowJoin) Unmarshal(c *jsonkit.Codec, data []byte, p *workflow.Join) error {
	var dto workflowJoinDTO
	if err := json.Unmarshal(data, &dto); err != nil {
		return err
	}
	p.Spawn = workflow.SpawnName(dto.Spawn)
	p.Collect = dto.Collect
	return nil
}

// Compile-time interface satisfaction check for WorkflowJoin, kept alongside
// the type so the contract remains visible in this file.
var _ jsonkit.ITypeCodec[workflow.Join] = WorkflowJoin{}

// This file owns the wire format for every workflow type that previously
// carried `json:"..."` struct tags. The workflow package itself stays free of
// any JSON-specific knowledge; each concrete type is (de)serialised through a
// dedicated codec here, registered into a fresh jsonkit.Codec by NewCodec.
//
// Conventions:
//
//   - One codec value per workflow type (e.g. WorkflowIf for workflow.If).
//     Each codec implements jsonkit.ITypeCodec[T] so it can be plugged into
//     CodecRegister. The codec MUST return only the inner JSON for the type
//     (no $type envelope); jsonkit wraps the result with the envelope on the
//     polymorphic path, and routes the bare result through directly otherwise.
//   - DTO structs are private (e.g. workflowIfDTO) and unexported. They carry
//     only json tags — no behavioural methods, no business logic.
//   - Nested Definition / Condition fields are marshalled recursively through
//     *jsonkit.Codec so the inner $type envelope is preserved. The DTO uses
//     json.RawMessage for those fields.
//   - json:"...,omitempty" matches the original struct tag when the field is a
//     pointer/collection; json:"...,omitzero" is used when the field is a
//     value type whose zero value should not appear on the wire.
//   - workflow.ProcessID / EventID (uuid.UUID) implement json.Marshaler /
//     json.Unmarshaler, so they round-trip as JSON strings inside the DTO
//     without an explicit string conversion.

///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

// Sequence

type WorkflowSequence struct{}

var _ jsonkit.ITypeCodec[workflow.Sequence] = WorkflowSequence{}

func (WorkflowSequence) Marshal(c *jsonkit.Codec, v workflow.Sequence) ([]byte, error) {
	if len(v) == 0 {
		return []byte("[]"), nil
	}
	elems := make([]json.RawMessage, len(v))
	for i, d := range v {
		if d == nil {
			elems[i] = []byte("null")
			continue
		}
		b, err := c.Marshal(jsonkit.Interface[workflow.Definition]{V: d})
		if err != nil {
			return nil, err
		}
		elems[i] = b
	}
	return json.Marshal(elems)
}

func (WorkflowSequence) Unmarshal(c *jsonkit.Codec, data []byte, p *workflow.Sequence) error {
	var raw []json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	out := make(workflow.Sequence, len(raw))
	for i, r := range raw {
		if len(r) == 0 || string(r) == "null" {
			continue
		}
		var d workflow.Definition
		if err := c.Unmarshal(r, &d); err != nil {
			return err
		}
		out[i] = d
	}
	*p = out
	return nil
}

///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

// If

type WorkflowIf struct{}

var _ jsonkit.ITypeCodec[workflow.If] = WorkflowIf{}

type workflowIfDTO struct {
	Cond json.RawMessage `json:"cond"`
	Then json.RawMessage `json:"then,omitempty"`
	Else json.RawMessage `json:"else,omitempty"`
}

func (WorkflowIf) Marshal(c *jsonkit.Codec, v workflow.If) ([]byte, error) {
	dto := workflowIfDTO{}
	if v.Cond != nil {
		b, err := c.Marshal(jsonkit.Interface[workflow.Condition]{V: v.Cond})
		if err != nil {
			return nil, err
		}
		dto.Cond = b
	}
	if v.Then != nil {
		b, err := c.Marshal(jsonkit.Interface[workflow.Definition]{V: v.Then})
		if err != nil {
			return nil, err
		}
		dto.Then = b
	}
	if v.Else != nil {
		b, err := c.Marshal(jsonkit.Interface[workflow.Definition]{V: v.Else})
		if err != nil {
			return nil, err
		}
		dto.Else = b
	}
	return json.Marshal(dto)
}

func (WorkflowIf) Unmarshal(c *jsonkit.Codec, data []byte, p *workflow.If) error {
	var dto workflowIfDTO
	if err := json.Unmarshal(data, &dto); err != nil {
		return err
	}
	if len(dto.Cond) > 0 && string(dto.Cond) != "null" {
		var cond workflow.Condition
		if err := c.Unmarshal(dto.Cond, &cond); err != nil {
			return err
		}
		p.Cond = cond
	}
	if len(dto.Then) > 0 && string(dto.Then) != "null" {
		var def workflow.Definition
		if err := c.Unmarshal(dto.Then, &def); err != nil {
			return err
		}
		p.Then = def
	}
	if len(dto.Else) > 0 && string(dto.Else) != "null" {
		var def workflow.Definition
		if err := c.Unmarshal(dto.Else, &def); err != nil {
			return err
		}
		p.Else = def
	}
	return nil
}

///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

type WorkflowSleep struct{}

var _ jsonkit.ITypeCodec[workflow.Sleep] = WorkflowSleep{}

type workflowSleepDTO struct {
	While json.RawMessage `json:"while,omitempty"`
	Until json.RawMessage `json:"until,omitempty"`
}

func (WorkflowSleep) Marshal(c *jsonkit.Codec, v workflow.Sleep) ([]byte, error) {
	dto := workflowSleepDTO{}
	if v.While != nil {
		b, err := c.Marshal(jsonkit.Interface[workflow.Condition]{V: v.While})
		if err != nil {
			return nil, err
		}
		dto.While = b
	}
	if v.Until != nil {
		b, err := c.Marshal(jsonkit.Interface[workflow.Condition]{V: v.Until})
		if err != nil {
			return nil, err
		}
		dto.Until = b
	}
	return json.Marshal(dto)
}

func (WorkflowSleep) Unmarshal(c *jsonkit.Codec, data []byte, p *workflow.Sleep) error {
	var dto workflowSleepDTO
	if err := json.Unmarshal(data, &dto); err != nil {
		return err
	}
	if len(dto.While) > 0 && string(dto.While) != "null" {
		var cond workflow.Condition
		if err := c.Unmarshal(dto.While, &cond); err != nil {
			return err
		}
		p.While = cond
	}
	if len(dto.Until) > 0 && string(dto.Until) != "null" {
		var cond workflow.Condition
		if err := c.Unmarshal(dto.Until, &cond); err != nil {
			return err
		}
		p.Until = cond
	}
	return nil
}

///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

// For

type WorkflowFor struct{}

var _ jsonkit.ITypeCodec[workflow.For] = WorkflowFor{}

type workflowForDTO struct {
	Init json.RawMessage `json:"init,omitempty"`
	Cond json.RawMessage `json:"cond"`
	Post json.RawMessage `json:"post,omitempty"`
	Do   json.RawMessage `json:"do,omitempty"`
}

func (WorkflowFor) Marshal(c *jsonkit.Codec, v workflow.For) ([]byte, error) {
	dto := workflowForDTO{}
	if v.Init != nil {
		b, err := c.Marshal(jsonkit.Interface[workflow.Definition]{V: v.Init})
		if err != nil {
			return nil, err
		}
		dto.Init = b
	}
	if v.Cond != nil {
		b, err := c.Marshal(jsonkit.Interface[workflow.Condition]{V: v.Cond})
		if err != nil {
			return nil, err
		}
		dto.Cond = b
	}
	if v.Post != nil {
		b, err := c.Marshal(jsonkit.Interface[workflow.Definition]{V: v.Post})
		if err != nil {
			return nil, err
		}
		dto.Post = b
	}
	if v.Do != nil {
		b, err := c.Marshal(jsonkit.Interface[workflow.Definition]{V: v.Do})
		if err != nil {
			return nil, err
		}
		dto.Do = b
	}
	return json.Marshal(dto)
}

func (WorkflowFor) Unmarshal(c *jsonkit.Codec, data []byte, p *workflow.For) error {
	var dto workflowForDTO
	if err := json.Unmarshal(data, &dto); err != nil {
		return err
	}
	if len(dto.Init) > 0 && string(dto.Init) != "null" {
		var def workflow.Definition
		if err := c.Unmarshal(dto.Init, &def); err != nil {
			return err
		}
		p.Init = def
	}
	if len(dto.Cond) > 0 && string(dto.Cond) != "null" {
		var cond workflow.Condition
		if err := c.Unmarshal(dto.Cond, &cond); err != nil {
			return err
		}
		p.Cond = cond
	}
	if len(dto.Post) > 0 && string(dto.Post) != "null" {
		var def workflow.Definition
		if err := c.Unmarshal(dto.Post, &def); err != nil {
			return err
		}
		p.Post = def
	}
	if len(dto.Do) > 0 && string(dto.Do) != "null" {
		var def workflow.Definition
		if err := c.Unmarshal(dto.Do, &def); err != nil {
			return err
		}
		p.Do = def
	}
	return nil
}

///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

// ForEach

type WorkflowForEach struct{}

var _ jsonkit.ITypeCodec[workflow.ForEach] = WorkflowForEach{}

type workflowForEachDTO struct {
	Over workflow.VarName `json:"over"`
	Do   json.RawMessage  `json:"do,omitempty"`

	K workflow.VarName `json:"key,omitempty"`
	V workflow.VarName `json:"value,omitempty"`
}

func (WorkflowForEach) Marshal(c *jsonkit.Codec, v workflow.ForEach) ([]byte, error) {
	dto := workflowForEachDTO{
		Over: v.Over,
		K:    v.K,
		V:    v.V,
	}
	if v.Do != nil {
		b, err := c.Marshal(jsonkit.Interface[workflow.Definition]{V: v.Do})
		if err != nil {
			return nil, err
		}
		dto.Do = b
	}
	return json.Marshal(dto)
}

func (WorkflowForEach) Unmarshal(c *jsonkit.Codec, data []byte, p *workflow.ForEach) error {
	var dto workflowForEachDTO
	if err := json.Unmarshal(data, &dto); err != nil {
		return err
	}
	p.Over = dto.Over
	p.K = dto.K
	p.V = dto.V
	if len(dto.Do) > 0 && string(dto.Do) != "null" {
		var def workflow.Definition
		if err := c.Unmarshal(dto.Do, &def); err != nil {
			return err
		}
		p.Do = def
	}
	return nil
}

///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

// DeclareVar

type WorkflowDeclareVar struct{}

var _ jsonkit.ITypeCodec[workflow.DeclareVar] = WorkflowDeclareVar{}

type workflowDeclareVarDTO struct {
	Name string `json:"name"`
	// Global is omitted while false, since declaring in the current variable
	// scope is the ordinary case and the zero value already means that.
	Global bool `json:"global,omitempty"`
}

func (WorkflowDeclareVar) Marshal(c *jsonkit.Codec, v workflow.DeclareVar) ([]byte, error) {
	return json.Marshal(workflowDeclareVarDTO{
		Name:   string(v.Name),
		Global: v.Global,
	})
}

func (WorkflowDeclareVar) Unmarshal(c *jsonkit.Codec, data []byte, p *workflow.DeclareVar) error {
	var dto workflowDeclareVarDTO
	if err := json.Unmarshal(data, &dto); err != nil {
		return err
	}
	p.Name = workflow.VarName(dto.Name)
	p.Global = dto.Global
	return nil
}

///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

// SetVar

type WorkflowSetVar struct{}

var _ jsonkit.ITypeCodec[workflow.SetVar] = WorkflowSetVar{}

type workflowSetVarDTO struct {
	Name  string          `json:"name"`
	Value json.RawMessage `json:"value"`
}

func (WorkflowSetVar) Marshal(c *jsonkit.Codec, v workflow.SetVar) ([]byte, error) {
	value, err := marshalValue(c, v.Value)
	if err != nil {
		return nil, err
	}
	return json.Marshal(workflowSetVarDTO{
		Name:  string(v.Name),
		Value: value,
	})
}

func (WorkflowSetVar) Unmarshal(c *jsonkit.Codec, data []byte, p *workflow.SetVar) error {
	var dto workflowSetVarDTO
	if err := json.Unmarshal(data, &dto); err != nil {
		return err
	}
	value, err := unmarshalValue(c, dto.Value)
	if err != nil {
		return err
	}
	p.Name = workflow.VarName(dto.Name)
	p.Value = value
	return nil
}

///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

// DeleteVar
//
// DeleteVar only carries the variable name; there is no value and no Global
// escape hatch — bindings are scope-local by construction, mirroring how a
// runtime `delete` would work. The wire format is a single string field.

type WorkflowDeleteVar struct{}

var _ jsonkit.ITypeCodec[workflow.DeleteVar] = WorkflowDeleteVar{}

type workflowDeleteVarDTO struct {
	Name string `json:"name"`
}

func (WorkflowDeleteVar) Marshal(c *jsonkit.Codec, v workflow.DeleteVar) ([]byte, error) {
	return json.Marshal(workflowDeleteVarDTO{
		Name: string(v.Name),
	})
}

func (WorkflowDeleteVar) Unmarshal(c *jsonkit.Codec, data []byte, p *workflow.DeleteVar) error {
	var dto workflowDeleteVarDTO
	if err := json.Unmarshal(data, &dto); err != nil {
		return err
	}
	p.Name = workflow.VarName(dto.Name)
	return nil
}

///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

// Increment
//
// Increment only carries the variable name; the increment amount is always one,
// so there is nothing else to put on the wire.

type WorkflowIncrement struct{}

var _ jsonkit.ITypeCodec[workflow.Increment] = WorkflowIncrement{}

type workflowIncrementDTO struct {
	Name string `json:"name"`
}

func (WorkflowIncrement) Marshal(c *jsonkit.Codec, v workflow.Increment) ([]byte, error) {
	return json.Marshal(workflowIncrementDTO{
		Name: string(v.Name),
	})
}

func (WorkflowIncrement) Unmarshal(c *jsonkit.Codec, data []byte, p *workflow.Increment) error {
	var dto workflowIncrementDTO
	if err := json.Unmarshal(data, &dto); err != nil {
		return err
	}
	p.Name = workflow.VarName(dto.Name)
	return nil
}

///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

// Spawn

type WorkflowSpawn struct{}

var _ jsonkit.ITypeCodec[workflow.Spawn] = WorkflowSpawn{}

type workflowSpawnDTO struct {
	Name       string              `json:"name"`
	Definition json.RawMessage     `json:"def"`
	Vars       workflow.VarMapping `json:"vars,omitzero"`
}

func (WorkflowSpawn) Marshal(c *jsonkit.Codec, v workflow.Spawn) ([]byte, error) {
	defBytes, err := c.Marshal(jsonkit.Interface[workflow.Definition]{V: v.Definition})
	if err != nil {
		return nil, err
	}
	return json.Marshal(workflowSpawnDTO{
		Name:       string(v.Name),
		Definition: defBytes,
		Vars:       v.Vars,
	})
}

func (WorkflowSpawn) Unmarshal(c *jsonkit.Codec, data []byte, p *workflow.Spawn) error {
	var dto workflowSpawnDTO
	if err := json.Unmarshal(data, &dto); err != nil {
		return err
	}
	var def workflow.Definition
	if err := c.Unmarshal(dto.Definition, &def); err != nil {
		return err
	}
	p.Name = workflow.SpawnName(dto.Name)
	p.Definition = def
	p.Vars = dto.Vars
	return nil
}

///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

// Defer

type WorkflowDefer struct{}

var _ jsonkit.ITypeCodec[workflow.Defer] = WorkflowDefer{}

type workflowDeferDTO struct {
	Definition json.RawMessage `json:"def,omitempty"`
}

func (WorkflowDefer) Marshal(c *jsonkit.Codec, v workflow.Defer) ([]byte, error) {
	var dto workflowDeferDTO
	if v.Definition != nil {
		b, err := c.Marshal(jsonkit.Interface[workflow.Definition]{V: v.Definition})
		if err != nil {
			return nil, err
		}
		dto.Definition = b
	}
	return json.Marshal(dto)
}

func (WorkflowDefer) Unmarshal(c *jsonkit.Codec, data []byte, p *workflow.Defer) error {
	var dto workflowDeferDTO
	if err := json.Unmarshal(data, &dto); err != nil {
		return err
	}
	p.Definition = nil
	if len(dto.Definition) > 0 && string(dto.Definition) != "null" {
		var def workflow.Definition
		if err := c.Unmarshal(dto.Definition, &def); err != nil {
			return err
		}
		p.Definition = def
	}
	return nil
}

///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

// Execute

type WorkflowExecute struct{}

var _ jsonkit.ITypeCodec[workflow.Execute] = WorkflowExecute{}

type workflowExecuteDTO struct {
	ParticipantID string   `json:"participant,omitempty"`
	ConditionID   string   `json:"condition,omitempty"`
	Input         []string `json:"input,omitempty"`
	Output        []string `json:"output,omitempty"`
}

func (WorkflowExecute) Marshal(c *jsonkit.Codec, v workflow.Execute) ([]byte, error) {
	return json.Marshal(workflowExecuteDTO{
		ParticipantID: string(v.Participant),
		ConditionID:   string(v.Condition),
		Input:         varNamesToStrings(v.Input),
		Output:        varNamesToStrings(v.Output),
	})
}

func (WorkflowExecute) Unmarshal(c *jsonkit.Codec, data []byte, p *workflow.Execute) error {
	var dto workflowExecuteDTO
	if err := json.Unmarshal(data, &dto); err != nil {
		return err
	}
	p.Participant = workflow.ParticipantID(dto.ParticipantID)
	p.Condition = workflow.ConditionID(dto.ConditionID)
	p.Input = varNamesFromStrings(dto.Input)
	p.Output = varNamesFromStrings(dto.Output)
	return nil
}

func varNamesToStrings(names []workflow.VarName) []string {
	if len(names) == 0 {
		return nil
	}
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = string(n)
	}
	return out
}

func varNamesFromStrings(names []string) []workflow.VarName {
	if len(names) == 0 {
		return nil
	}
	out := make([]workflow.VarName, len(names))
	for i, n := range names {
		out[i] = workflow.VarName(n)
	}
	return out
}

///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

// -- Events --

// EventCompleted

type WorkflowEventCompleted struct{}

var _ jsonkit.ITypeCodec[workflow.EventCompleted] = WorkflowEventCompleted{}

type workflowEventCompletedDTO struct {
	EventID   workflow.EventID   `json:"event_id"`
	ProcessID workflow.ProcessID `json:"process_id"`
	Timestamp time.Time          `json:"timestamp"`
}

func (WorkflowEventCompleted) Marshal(c *jsonkit.Codec, v workflow.EventCompleted) ([]byte, error) {
	return json.Marshal(workflowEventCompletedDTO{
		EventID:   v.EventID,
		ProcessID: v.ProcessID,
		Timestamp: v.Timestamp,
	})
}

func (WorkflowEventCompleted) Unmarshal(c *jsonkit.Codec, data []byte, p *workflow.EventCompleted) error {
	var dto workflowEventCompletedDTO
	if err := json.Unmarshal(data, &dto); err != nil {
		return err
	}
	p.EventID = dto.EventID
	p.ProcessID = dto.ProcessID
	p.Timestamp = dto.Timestamp
	return nil
}

// EventTerminated

type WorkflowEventTerminated struct{}

var _ jsonkit.ITypeCodec[workflow.EventTerminated] = WorkflowEventTerminated{}

type workflowEventTerminatedDTO struct {
	EventID   workflow.EventID   `json:"event_id"`
	ProcessID workflow.ProcessID `json:"process_id"`
	Timestamp time.Time          `json:"timestamp"`
}

func (WorkflowEventTerminated) Marshal(c *jsonkit.Codec, v workflow.EventTerminated) ([]byte, error) {
	return json.Marshal(workflowEventTerminatedDTO{
		EventID:   v.EventID,
		ProcessID: v.ProcessID,
		Timestamp: v.Timestamp,
	})
}

func (WorkflowEventTerminated) Unmarshal(c *jsonkit.Codec, data []byte, p *workflow.EventTerminated) error {
	var dto workflowEventTerminatedDTO
	if err := json.Unmarshal(data, &dto); err != nil {
		return err
	}
	p.EventID = dto.EventID
	p.ProcessID = dto.ProcessID
	p.Timestamp = dto.Timestamp
	return nil
}

// VarEvent

type WorkflowEventDeclareVar struct{}

var _ jsonkit.ITypeCodec[workflow.EventDeclareVar] = WorkflowEventDeclareVar{}

type workflowEventDeclareVarDTO struct {
	EventID   workflow.EventID   `json:"event_id"`
	ProcessID workflow.ProcessID `json:"process_id"`
	Timestamp time.Time          `json:"timestamp"`
	Path      PathDTO            `json:"path,omitempty"`
	Name      string             `json:"name"`
}

func (WorkflowEventDeclareVar) Marshal(c *jsonkit.Codec, v workflow.EventDeclareVar) ([]byte, error) {
	return json.Marshal(workflowEventDeclareVarDTO{
		EventID:   v.EventID,
		ProcessID: v.ProcessID,
		Timestamp: v.Timestamp,
		Path:      ToPathDTO(v.Path),
		Name:      string(v.Name),
	})
}

func (WorkflowEventDeclareVar) Unmarshal(c *jsonkit.Codec, data []byte, p *workflow.EventDeclareVar) error {
	var dto workflowEventDeclareVarDTO
	if err := json.Unmarshal(data, &dto); err != nil {
		return err
	}
	p.EventID = dto.EventID
	p.ProcessID = dto.ProcessID
	p.Timestamp = dto.Timestamp
	p.Path = ToPath(dto.Path)
	p.Name = workflow.VarName(dto.Name)
	return nil
}

type WorkflowEventSetVar struct{}

var _ jsonkit.ITypeCodec[workflow.EventSetVar] = WorkflowEventSetVar{}

type workflowEventSetVarDTO struct {
	EventID   workflow.EventID   `json:"event_id"`
	ProcessID workflow.ProcessID `json:"process_id"`
	Timestamp time.Time          `json:"timestamp"`
	Path      PathDTO            `json:"path,omitempty"`
	Name      string             `json:"name"`
	Value     json.RawMessage    `json:"value,omitempty"`
}

func (WorkflowEventSetVar) Marshal(c *jsonkit.Codec, v workflow.EventSetVar) ([]byte, error) {
	dto := workflowEventSetVarDTO{
		EventID:   v.EventID,
		ProcessID: v.ProcessID,
		Timestamp: v.Timestamp,
		Path:      ToPathDTO(v.Path),
		Name:      string(v.Name),
	}
	// A nil Value is left out, the way omitempty leaves out a nil interface.
	if v.Value != nil {
		value, err := marshalValue(c, v.Value)
		if err != nil {
			return nil, err
		}
		dto.Value = value
	}
	return json.Marshal(dto)
}

func (WorkflowEventSetVar) Unmarshal(c *jsonkit.Codec, data []byte, p *workflow.EventSetVar) error {
	var dto workflowEventSetVarDTO
	if err := json.Unmarshal(data, &dto); err != nil {
		return err
	}
	value, err := unmarshalValue(c, dto.Value)
	if err != nil {
		return err
	}
	p.EventID = dto.EventID
	p.ProcessID = dto.ProcessID
	p.Timestamp = dto.Timestamp
	p.Path = ToPath(dto.Path)
	p.Name = workflow.VarName(dto.Name)
	p.Value = value
	return nil
}

type WorkflowEventDeleteVar struct{}

var _ jsonkit.ITypeCodec[workflow.EventDeleteVar] = WorkflowEventDeleteVar{}

type workflowEventDeleteVarDTO struct {
	EventID   workflow.EventID   `json:"event_id"`
	ProcessID workflow.ProcessID `json:"process_id"`
	Timestamp time.Time          `json:"timestamp"`
	Path      PathDTO            `json:"path,omitempty"`
	Name      string             `json:"name"`
}

func (WorkflowEventDeleteVar) Marshal(c *jsonkit.Codec, v workflow.EventDeleteVar) ([]byte, error) {
	return json.Marshal(workflowEventDeleteVarDTO{
		EventID:   v.EventID,
		ProcessID: v.ProcessID,
		Timestamp: v.Timestamp,
		Path:      ToPathDTO(v.Path),
		Name:      string(v.Name),
	})
}

func (WorkflowEventDeleteVar) Unmarshal(c *jsonkit.Codec, data []byte, p *workflow.EventDeleteVar) error {
	var dto workflowEventDeleteVarDTO
	if err := json.Unmarshal(data, &dto); err != nil {
		return err
	}
	p.EventID = dto.EventID
	p.ProcessID = dto.ProcessID
	p.Timestamp = dto.Timestamp
	p.Path = ToPath(dto.Path)
	p.Name = workflow.VarName(dto.Name)
	return nil
}

// ExecuteParticipantEvent

type WorkflowEventParticipant struct{}

var _ jsonkit.ITypeCodec[workflow.EventParticipant] = WorkflowEventParticipant{}

type workflowExecuteParticipantEventDTO struct {
	EventID       workflow.EventID   `json:"event_id"`
	ProcessID     workflow.ProcessID `json:"process_id"`
	Timestamp     time.Time          `json:"timestamp"`
	ParticipantID string             `json:"participant_id,omitempty"`
	Path          PathDTO            `json:"path,omitempty"`
	Input         []json.RawMessage  `json:"input,omitempty"`
	Output        []json.RawMessage  `json:"output,omitempty"`
	Definition    json.RawMessage    `json:"definition,omitempty"`
}

func (WorkflowEventParticipant) Marshal(c *jsonkit.Codec, v workflow.EventParticipant) ([]byte, error) {
	input, err := marshalValues(c, v.Input)
	if err != nil {
		return nil, err
	}
	output, err := marshalValues(c, v.Output)
	if err != nil {
		return nil, err
	}
	dto := workflowExecuteParticipantEventDTO{
		EventID:       v.EventID,
		ProcessID:     v.ProcessID,
		Timestamp:     v.Timestamp,
		ParticipantID: string(v.ParticipantID),
		Path:          ToPathDTO(v.Path),
		Input:         input,
		Output:        output,
	}
	if v.Definition != nil {
		b, err := c.Marshal(jsonkit.Interface[workflow.Definition]{V: v.Definition})
		if err != nil {
			return nil, err
		}
		dto.Definition = b
	}
	return json.Marshal(dto)
}

func (WorkflowEventParticipant) Unmarshal(c *jsonkit.Codec, data []byte, p *workflow.EventParticipant) error {
	var dto workflowExecuteParticipantEventDTO
	if err := json.Unmarshal(data, &dto); err != nil {
		return err
	}
	input, err := unmarshalValues(c, dto.Input)
	if err != nil {
		return err
	}
	output, err := unmarshalValues(c, dto.Output)
	if err != nil {
		return err
	}
	p.EventID = dto.EventID
	p.ProcessID = dto.ProcessID
	p.Timestamp = dto.Timestamp
	p.ParticipantID = workflow.ParticipantID(dto.ParticipantID)
	p.Path = ToPath(dto.Path)
	p.Input = input
	p.Output = output
	if len(dto.Definition) > 0 && string(dto.Definition) != "null" {
		var def workflow.Definition
		if err := c.Unmarshal(dto.Definition, &def); err != nil {
			return err
		}
		p.Definition = def
	}
	return nil
}

// ExecuteConditionEvent

type WorkflowEventCondition struct{}

var _ jsonkit.ITypeCodec[workflow.EventParticipant] = WorkflowEventParticipant{}

type workflowExecuteConditionEventDTO struct {
	EventID     workflow.EventID   `json:"event_id"`
	ProcessID   workflow.ProcessID `json:"process_id"`
	ConditionID string             `json:"condition_id,omitempty"`
	Path        PathDTO            `json:"path,omitempty"`
	Input       []json.RawMessage  `json:"input"`
	Answer      bool               `json:"answer"`
	Timestamp   time.Time          `json:"timestamp"`
}

func (WorkflowEventCondition) Marshal(c *jsonkit.Codec, v workflow.EventCondition) ([]byte, error) {
	input, err := marshalValues(c, v.Input)
	if err != nil {
		return nil, err
	}
	return json.Marshal(workflowExecuteConditionEventDTO{
		EventID:     v.EventID,
		ProcessID:   v.ProcessID,
		ConditionID: string(v.ConditionID),
		Path:        ToPathDTO(v.Path),
		Input:       input,
		Answer:      v.Answer,
		Timestamp:   v.Timestamp,
	})
}

func (WorkflowEventCondition) Unmarshal(c *jsonkit.Codec, data []byte, p *workflow.EventCondition) error {
	var dto workflowExecuteConditionEventDTO
	if err := json.Unmarshal(data, &dto); err != nil {
		return err
	}
	input, err := unmarshalValues(c, dto.Input)
	if err != nil {
		return err
	}
	p.EventID = dto.EventID
	p.ProcessID = dto.ProcessID
	p.ConditionID = workflow.ConditionID(dto.ConditionID)
	p.Path = ToPath(dto.Path)
	p.Input = input
	p.Answer = dto.Answer
	p.Timestamp = dto.Timestamp
	return nil
}

// UseDefinitionEvent

type WorkflowEventUseDefinition struct{}

var _ jsonkit.ITypeCodec[workflow.EventUseDefinition] = WorkflowEventUseDefinition{}

type workflowUseDefinitionEventDTO struct {
	EventID    workflow.EventID   `json:"event_id"`
	ProcessID  workflow.ProcessID `json:"process_id"`
	Timestamp  time.Time          `json:"timestamp"`
	Definition json.RawMessage    `json:"definition"`
}

func (WorkflowEventUseDefinition) Marshal(c *jsonkit.Codec, v workflow.EventUseDefinition) ([]byte, error) {
	defBytes, err := c.Marshal(jsonkit.Interface[workflow.Definition]{V: v.Definition})
	if err != nil {
		return nil, err
	}
	return json.Marshal(workflowUseDefinitionEventDTO{
		EventID:    v.EventID,
		ProcessID:  v.ProcessID,
		Timestamp:  v.Timestamp,
		Definition: defBytes,
	})
}

func (WorkflowEventUseDefinition) Unmarshal(c *jsonkit.Codec, data []byte, p *workflow.EventUseDefinition) error {
	var dto workflowUseDefinitionEventDTO
	if err := json.Unmarshal(data, &dto); err != nil {
		return err
	}
	var def workflow.Definition
	if err := c.Unmarshal(dto.Definition, &def); err != nil {
		return err
	}
	p.EventID = dto.EventID
	p.ProcessID = dto.ProcessID
	p.Timestamp = dto.Timestamp
	p.Definition = def
	return nil
}

// SpawnEvent

type WorkflowEventSpawn struct{}

var _ jsonkit.ITypeCodec[workflow.EventSpawn] = WorkflowEventSpawn{}

type workflowSpawnEventDTO struct {
	EventID   workflow.EventID   `json:"event_id"`
	ProcessID workflow.ProcessID `json:"process_id"`
	ChildID   workflow.ProcessID `json:"child_id"`
	Name      string             `json:"name,omitzero"`
	Timestamp time.Time          `json:"timestamp"`
	Path      PathDTO            `json:"path,omitempty"`
}

func (WorkflowEventSpawn) Marshal(c *jsonkit.Codec, v workflow.EventSpawn) ([]byte, error) {
	return json.Marshal(workflowSpawnEventDTO{
		EventID:   v.EventID,
		ProcessID: v.ProcessID,
		ChildID:   v.ChildID,
		Name:      string(v.Name),
		Timestamp: v.Timestamp,
		Path:      ToPathDTO(v.Path),
	})
}

func (WorkflowEventSpawn) Unmarshal(c *jsonkit.Codec, data []byte, p *workflow.EventSpawn) error {
	var dto workflowSpawnEventDTO
	if err := json.Unmarshal(data, &dto); err != nil {
		return err
	}
	p.EventID = dto.EventID
	p.ProcessID = dto.ProcessID
	p.ChildID = dto.ChildID
	p.Name = workflow.SpawnName(dto.Name)
	p.Timestamp = dto.Timestamp
	p.Path = ToPath(dto.Path)
	return nil
}

// JoinEvent

type WorkflowEventJoin struct{}

var _ jsonkit.ITypeCodec[workflow.EventJoin] = WorkflowEventJoin{}

type workflowJoinEventDTO struct {
	EventID   workflow.EventID     `json:"event_id"`
	ProcessID workflow.ProcessID   `json:"process_id"`
	Timestamp time.Time            `json:"timestamp"`
	Children  []workflow.ProcessID `json:"children,omitzero"`
	Path      PathDTO              `json:"path,omitempty"`
}

func (WorkflowEventJoin) Marshal(c *jsonkit.Codec, v workflow.EventJoin) ([]byte, error) {
	return json.Marshal(workflowJoinEventDTO{
		EventID:   v.EventID,
		ProcessID: v.ProcessID,
		Timestamp: v.Timestamp,
		Children:  v.Children,
		Path:      ToPathDTO(v.Path),
	})
}

func (WorkflowEventJoin) Unmarshal(c *jsonkit.Codec, data []byte, p *workflow.EventJoin) error {
	var dto workflowJoinEventDTO
	if err := json.Unmarshal(data, &dto); err != nil {
		return err
	}
	p.EventID = dto.EventID
	p.ProcessID = dto.ProcessID
	p.Timestamp = dto.Timestamp
	p.Children = dto.Children
	p.Path = ToPath(dto.Path)
	return nil
}

// SleepCompletedEvent

type WorkflowEventSleepCompleted struct{}

var _ jsonkit.ITypeCodec[workflow.EventSleepCompleted] = WorkflowEventSleepCompleted{}

type workflowSleepCompletedEventDTO struct {
	EventID   workflow.EventID   `json:"event_id"`
	ProcessID workflow.ProcessID `json:"process_id"`
	Timestamp time.Time          `json:"timestamp"`
	Path      PathDTO            `json:"path,omitempty"`
}

func (WorkflowEventSleepCompleted) Marshal(c *jsonkit.Codec, v workflow.EventSleepCompleted) ([]byte, error) {
	return json.Marshal(workflowSleepCompletedEventDTO{
		EventID:   v.EventID,
		ProcessID: v.ProcessID,
		Timestamp: v.Timestamp,
		Path:      ToPathDTO(v.Path),
	})
}

func (WorkflowEventSleepCompleted) Unmarshal(c *jsonkit.Codec, data []byte, p *workflow.EventSleepCompleted) error {
	var dto workflowSleepCompletedEventDTO
	if err := json.Unmarshal(data, &dto); err != nil {
		return err
	}
	p.EventID = dto.EventID
	p.ProcessID = dto.ProcessID
	p.Timestamp = dto.Timestamp
	p.Path = ToPath(dto.Path)
	return nil
}

// DeferEvent

type WorkflowEventDefer struct{}

var _ jsonkit.ITypeCodec[workflow.EventDefer] = WorkflowEventDefer{}

type workflowDeferEventDTO struct {
	EventID    workflow.EventID   `json:"event_id"`
	ProcessID  workflow.ProcessID `json:"process_id"`
	Timestamp  time.Time          `json:"timestamp"`
	Path       PathDTO            `json:"path,omitempty"`
	Definition json.RawMessage    `json:"definition"`
}

func (WorkflowEventDefer) Marshal(c *jsonkit.Codec, v workflow.EventDefer) ([]byte, error) {
	defBytes, err := c.Marshal(jsonkit.Interface[workflow.Definition]{V: v.Definition})
	if err != nil {
		return nil, err
	}
	return json.Marshal(workflowDeferEventDTO{
		EventID:    v.EventID,
		ProcessID:  v.ProcessID,
		Timestamp:  v.Timestamp,
		Path:       ToPathDTO(v.Path),
		Definition: defBytes,
	})
}

func (WorkflowEventDefer) Unmarshal(c *jsonkit.Codec, data []byte, p *workflow.EventDefer) error {
	var dto workflowDeferEventDTO
	if err := json.Unmarshal(data, &dto); err != nil {
		return err
	}
	var def workflow.Definition
	if err := c.Unmarshal(dto.Definition, &def); err != nil {
		return err
	}
	p.EventID = dto.EventID
	p.ProcessID = dto.ProcessID
	p.Timestamp = dto.Timestamp
	p.Path = ToPath(dto.Path)
	p.Definition = def
	return nil
}

// ForEachIterationEvent

type WorkflowEventForEachIteration struct{}

var _ jsonkit.ITypeCodec[workflow.EventForEachIteration] = WorkflowEventForEachIteration{}

type workflowForEachIterationEventDTO struct {
	EventID   workflow.EventID   `json:"event_id"`
	ProcessID workflow.ProcessID `json:"process_id"`
	Timestamp time.Time          `json:"timestamp"`
	Path      PathDTO            `json:"path,omitempty"`
	Over      workflow.VarName   `json:"over"`
	K         workflow.VarName   `json:"key,omitempty"`
	V         workflow.VarName   `json:"value,omitempty"`
}

func (WorkflowEventForEachIteration) Marshal(c *jsonkit.Codec, v workflow.EventForEachIteration) ([]byte, error) {
	return json.Marshal(workflowForEachIterationEventDTO{
		EventID:   v.EventID,
		ProcessID: v.ProcessID,
		Timestamp: v.Timestamp,
		Path:      ToPathDTO(v.Path),
		Over:      v.Over,
		K:         v.K,
		V:         v.V,
	})
}

func (WorkflowEventForEachIteration) Unmarshal(c *jsonkit.Codec, data []byte, p *workflow.EventForEachIteration) error {
	var dto workflowForEachIterationEventDTO
	if err := json.Unmarshal(data, &dto); err != nil {
		return err
	}
	p.EventID = dto.EventID
	p.ProcessID = dto.ProcessID
	p.Timestamp = dto.Timestamp
	p.Path = ToPath(dto.Path)
	p.Over = dto.Over
	p.K = dto.K
	p.V = dto.V
	return nil
}

// ErrorEvent

type WorkflowEventError struct{}

var _ jsonkit.ITypeCodec[workflow.EventError] = WorkflowEventError{}

type workflowErrorEventDTO struct {
	EventID       workflow.EventID   `json:"event_id"`
	ProcessID     workflow.ProcessID `json:"process_id"`
	Timestamp     time.Time          `json:"timestamp"`
	ParticipantID string             `json:"participant_id,omitempty"`
	Path          PathDTO            `json:"path,omitempty"`
	Error         string             `json:"error"`
}

func (WorkflowEventError) Marshal(c *jsonkit.Codec, v workflow.EventError) ([]byte, error) {
	return json.Marshal(workflowErrorEventDTO{
		EventID:       v.EventID,
		ProcessID:     v.ProcessID,
		Timestamp:     v.Timestamp,
		ParticipantID: string(v.ParticipantID),
		Path:          ToPathDTO(v.Path),
		Error:         v.Error,
	})
}

func (WorkflowEventError) Unmarshal(c *jsonkit.Codec, data []byte, p *workflow.EventError) error {
	var dto workflowErrorEventDTO
	if err := json.Unmarshal(data, &dto); err != nil {
		return err
	}
	p.EventID = dto.EventID
	p.ProcessID = dto.ProcessID
	p.Timestamp = dto.Timestamp
	p.ParticipantID = workflow.ParticipantID(dto.ParticipantID)
	p.Path = ToPath(dto.Path)
	p.Error = dto.Error
	return nil
}

///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

// -- Schedule types (not Definitions/Events/Conditions) --

// Schedule

type WorkflowExecutionRequest struct{}

var _ jsonkit.ITypeCodec[workflow.ExecutionRequest] = WorkflowExecutionRequest{}

type workflowScheduleDTO struct {
	ProcessID    workflow.ProcessID `json:"process_id"`
	StartTime    time.Time          `json:"start,omitzero"`
	CreatedAt    time.Time          `json:"created_at,omitzero"`
	FailureCount int                `json:"failure_count,omitzero"`
}

func (WorkflowExecutionRequest) Marshal(c *jsonkit.Codec, v workflow.ExecutionRequest) ([]byte, error) {
	return json.Marshal(workflowScheduleDTO{
		ProcessID:    v.ProcessID,
		StartTime:    v.StartTime,
		CreatedAt:    v.CreatedAt,
		FailureCount: v.FailureCount,
	})
}

func (WorkflowExecutionRequest) Unmarshal(c *jsonkit.Codec, data []byte, p *workflow.ExecutionRequest) error {
	var dto workflowScheduleDTO
	if err := json.Unmarshal(data, &dto); err != nil {
		return err
	}
	p.ProcessID = dto.ProcessID
	p.StartTime = dto.StartTime
	p.CreatedAt = dto.CreatedAt
	p.FailureCount = dto.FailureCount
	return nil
}

// Notification is a polymorphic interface; each concrete implementation
// gets its own registered codec so jsonkit's $type envelope can dispatch
// Marshal/Unmarshal to the right DTO shape on the wire.

type WorkflowProcessSchedule struct{}

var _ jsonkit.ITypeCodec[workflow.ProcessSchedule] = WorkflowProcessSchedule{}

type workflowProcessScheduleDTO struct {
	ProcessID workflow.ProcessID `json:"process_id"`
}

func (WorkflowProcessSchedule) Marshal(c *jsonkit.Codec, v workflow.ProcessSchedule) ([]byte, error) {
	return json.Marshal(workflowProcessScheduleDTO{ProcessID: v.ProcessID})
}

func (WorkflowProcessSchedule) Unmarshal(c *jsonkit.Codec, data []byte, p *workflow.ProcessSchedule) error {
	var dto workflowProcessScheduleDTO
	if err := json.Unmarshal(data, &dto); err != nil {
		return err
	}
	p.ProcessID = dto.ProcessID
	return nil
}

type WorkflowEventProcessCancel struct{}

var _ jsonkit.ITypeCodec[workflow.ProcessCancel] = WorkflowEventProcessCancel{}

type workflowProcessCancelDTO struct {
	ProcessID workflow.ProcessID `json:"process_id"`
}

func (WorkflowEventProcessCancel) Marshal(c *jsonkit.Codec, v workflow.ProcessCancel) ([]byte, error) {
	return json.Marshal(workflowProcessCancelDTO{ProcessID: v.ProcessID})
}

func (WorkflowEventProcessCancel) Unmarshal(c *jsonkit.Codec, data []byte, p *workflow.ProcessCancel) error {
	var dto workflowProcessCancelDTO
	if err := json.Unmarshal(data, &dto); err != nil {
		return err
	}
	p.ProcessID = dto.ProcessID
	return nil
}

// ToPath converts the wire representation of a workflow.Path back into the
// domain type.
//
// A nil DTO maps back to a nil Path, so that an absent "path" field
// round-trips as the zero value instead of an empty slice.
func ToPath(dto PathDTO) workflow.Path {
	if dto == nil {
		return nil
	}
	var p = make(workflow.Path, 0, len(dto))
	for _, s := range dto {
		p = append(p, workflow.Scope{Name: s.Name, Var: s.Var})
	}
	return p
}

// ToPathDTO converts a workflow.Path into its wire representation.
//
// A nil Path maps to a nil DTO, so that the zero value is omitted from the
// output instead of being rendered as an empty JSON array.
func ToPathDTO(p workflow.Path) PathDTO {
	if p == nil {
		return nil
	}
	var dto = make(PathDTO, 0, len(p))
	for _, s := range p {
		dto = append(dto, ScopeDTO{Name: s.Name, Var: s.Var})
	}
	return dto
}

// PathDTO is the wire representation of a workflow.Path:
// a JSON array of its Scopes, from the root.
//
// The "var" marks are part of the Path, as they tell the variable scopes of the location,
// and the variable events rely on them to tell the scope of the variable they are about.
type PathDTO []ScopeDTO

// ScopeDTO is the wire representation of a workflow.Scope.
//
//	{"name": "for", "var": true}
//
// "var" is omitted for a Scope which is not a variable scope.
type ScopeDTO struct {
	Name string `json:"name"`
	Var  bool   `json:"var,omitempty"`
}

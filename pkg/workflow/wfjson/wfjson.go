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
// concrete type by its @type tag without depending on the global registry
// being initialised.
//
// Workflow.Runtimes fall back to this when their Codec field is nil, so users
// can encode/decode any workflow value without additional setup.
func NewCodec() *jsonkit.Codec {
	var c jsonkit.Codec

	// Conditions: wftemplate.Condition is a named primitive (string)
	// whose @type envelope is handled by jsonkit's reflect-based path,
	// so CodecRegisterTypeID is enough. ExecuteCondition has a custom
	// codec (DTO shape diverges from the workflow struct shape).
	jsonkit.CodecRegisterTypeID[wftemplate.Condition](&c, "workflow::template::condition")

	// Definitions: every concrete type gets a custom codec because the
	// wire format (snake_case, DTO shape, nested @type envelopes) is
	// owned by this package and may diverge from the workflow struct
	// shape over time.
	jsonkit.CodecRegister[workflow.Sequence](&c, "workflow::sequence", WorkflowSequence{})
	jsonkit.CodecRegister[workflow.If](&c, "workflow::if", WorkflowIf{})
	jsonkit.CodecRegister[workflow.Sleep](&c, "workflow::sleep", WorkflowSleep{})
	jsonkit.CodecRegister[workflow.For](&c, "workflow::for", WorkflowFor{})
	jsonkit.CodecRegister[workflow.ForEach](&c, "workflow::foreach", WorkflowForEach{})
	// Break carries no state, so the reflect-based path is enough: it needs a
	// wire identity, not a shape.
	jsonkit.CodecRegisterTypeID[workflow.Break](&c, "workflow::break")
	jsonkit.CodecRegister[workflow.SetVar](&c, "workflow::var::set", WorkflowSetVar{})
	jsonkit.CodecRegister[workflow.DeclareVar](&c, "workflow::var::declare", WorkflowDeclareVar{})
	jsonkit.CodecRegister[workflow.DeleteVar](&c, "workflow::var::delete", WorkflowDeleteVar{})
	jsonkit.CodecRegister[workflow.Increment](&c, "workflow::op::increment", WorkflowIncrement{})
	jsonkit.CodecRegister[workflow.Spawn](&c, "workflow::spawn", WorkflowSpawn{})
	jsonkit.CodecRegister[workflow.ExecuteParticipant](&c, "workflow::participant", WorkflowExecuteParticipant{})
	jsonkit.CodecRegister[workflow.ExecuteCondition](&c, "workflow::condition", WorkflowExecuteCondition{})
	jsonkit.CodecRegister[workflow.Join](&c, "workflow::join", WorkflowJoin{})

	// Events: same treatment — custom codecs own the wire format.
	jsonkit.CodecRegister[workflow.EventCompleted](&c, "workflow::event::completed", WorkflowEventCompleted{})
	jsonkit.CodecRegister[workflow.EventTerminated](&c, "workflow::event::terminated", WorkflowEventTerminated{})
	jsonkit.CodecRegister[workflow.EventDeclareVar](&c, "workflow::event::var::declare", WorkflowEventDeclareVar{})
	jsonkit.CodecRegister[workflow.EventSetVar](&c, "workflow::event::var::set", WorkflowEventSetVar{})
	jsonkit.CodecRegister[workflow.EventDeleteVar](&c, "workflow::event::var::delete", WorkflowEventDeleteVar{})
	jsonkit.CodecRegister[workflow.EventParticipant](&c, "workflow::event::participant", WorkflowEventParticipant{})
	jsonkit.CodecRegister[workflow.EventCondition](&c, "workflow::event::condition", WorkflowEventCondition{})
	jsonkit.CodecRegister[workflow.EventUseDefinition](&c, "workflow::event::use-definition", WorkflowEventUseDefinition{})
	jsonkit.CodecRegister[workflow.EventSpawn](&c, "workflow::event::spawn", WorkflowEventSpawn{})
	jsonkit.CodecRegister[workflow.EventJoin](&c, "workflow::event::join", WorkflowEventJoin{})

	// Schedule-side types: not part of Definition/Condition/Event but
	// still persisted across the runtime, so they need a stable wire format too.
	jsonkit.CodecRegister[workflow.ProcessExecution](&c, "workflow::execution", WorkflowSchedule{})
	jsonkit.CodecRegister[workflow.ProcessSchedule](&c, "workflow::schedule", WorkflowEventProcessSchedule{})
	jsonkit.CodecRegister[workflow.ProcessCancel](&c, "workflow::cancel", WorkflowEventProcessCancel{})

	return &c
}

// WorkflowJoin is the codec for workflow.Join. It is kept here (rather than in
// dto.go) so its declaration stays next to the original example that anchored
// the per-type codec pattern.
type WorkflowJoin struct{}

type workflowJoinDTO struct {
	SpawnName string              `json:"spawn_name,omitzero"`
	Collect   workflow.VarMapping `json:"collect,omitzero"`
}

func (WorkflowJoin) Marshal(c *jsonkit.Codec, v workflow.Join) ([]byte, error) {
	return json.Marshal(workflowJoinDTO{
		SpawnName: string(v.SpawnName),
		Collect:   v.Collect,
	})
}

func (WorkflowJoin) Unmarshal(c *jsonkit.Codec, data []byte, p *workflow.Join) error {
	var dto workflowJoinDTO
	if err := json.Unmarshal(data, &dto); err != nil {
		return err
	}
	p.SpawnName = workflow.SpawnName(dto.SpawnName)
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
//     (no @type envelope); jsonkit wraps the result with the envelope on the
//     polymorphic path, and routes the bare result through directly otherwise.
//   - DTO structs are private (e.g. workflowIfDTO) and unexported. They carry
//     only json tags — no behavioural methods, no business logic.
//   - Nested Definition / Condition fields are marshalled recursively through
//     *jsonkit.Codec so the inner @type envelope is preserved. The DTO uses
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
		b, err := c.Marshal(d)
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
		b, err := c.Marshal(v.Cond)
		if err != nil {
			return nil, err
		}
		dto.Cond = b
	}
	if v.Then != nil {
		b, err := c.Marshal(v.Then)
		if err != nil {
			return nil, err
		}
		dto.Then = b
	}
	if v.Else != nil {
		b, err := c.Marshal(v.Else)
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
		b, err := c.Marshal(v.While)
		if err != nil {
			return nil, err
		}
		dto.While = b
	}
	if v.Until != nil {
		b, err := c.Marshal(v.Until)
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
		b, err := c.Marshal(v.Init)
		if err != nil {
			return nil, err
		}
		dto.Init = b
	}
	if v.Cond != nil {
		b, err := c.Marshal(v.Cond)
		if err != nil {
			return nil, err
		}
		dto.Cond = b
	}
	if v.Post != nil {
		b, err := c.Marshal(v.Post)
		if err != nil {
			return nil, err
		}
		dto.Post = b
	}
	if v.Do != nil {
		b, err := c.Marshal(v.Do)
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
		b, err := c.Marshal(v.Do)
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
	Name  string `json:"name"`
	Value any    `json:"value"`
}

func (WorkflowSetVar) Marshal(c *jsonkit.Codec, v workflow.SetVar) ([]byte, error) {
	return json.Marshal(workflowSetVarDTO{
		Name:  string(v.Name),
		Value: v.Value,
	})
}

func (WorkflowSetVar) Unmarshal(c *jsonkit.Codec, data []byte, p *workflow.SetVar) error {
	var dto workflowSetVarDTO
	if err := json.Unmarshal(data, &dto); err != nil {
		return err
	}
	p.Name = workflow.VarName(dto.Name)
	p.Value = dto.Value
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
	defBytes, err := c.Marshal(v.Definition)
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

// ExecuteParticipant

type WorkflowExecuteParticipant struct{}

var _ jsonkit.ITypeCodec[workflow.ExecuteParticipant] = WorkflowExecuteParticipant{}

type workflowExecuteParticipantDTO struct {
	ID     string   `json:"id"`
	Input  []string `json:"input,omitempty"`
	Output []string `json:"output,omitempty"`
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

func (WorkflowExecuteParticipant) Marshal(c *jsonkit.Codec, v workflow.ExecuteParticipant) ([]byte, error) {
	return json.Marshal(workflowExecuteParticipantDTO{
		ID:     string(v.ID),
		Input:  varNamesToStrings(v.Input),
		Output: varNamesToStrings(v.Output),
	})
}

func (WorkflowExecuteParticipant) Unmarshal(c *jsonkit.Codec, data []byte, p *workflow.ExecuteParticipant) error {
	var dto workflowExecuteParticipantDTO
	if err := json.Unmarshal(data, &dto); err != nil {
		return err
	}
	p.ID = workflow.ParticipantID(dto.ID)
	p.Input = varNamesFromStrings(dto.Input)
	p.Output = varNamesFromStrings(dto.Output)
	return nil
}

///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

// ExecuteCondition

type WorkflowExecuteCondition struct{}

var _ jsonkit.ITypeCodec[workflow.ExecuteCondition] = WorkflowExecuteCondition{}

type workflowExecuteConditionDTO struct {
	ID    string   `json:"id"`
	Input []string `json:"input,omitempty"`
}

func (WorkflowExecuteCondition) Marshal(c *jsonkit.Codec, v workflow.ExecuteCondition) ([]byte, error) {
	return json.Marshal(workflowExecuteConditionDTO{
		ID:    string(v.ID),
		Input: varNamesToStrings(v.Input),
	})
}

func (WorkflowExecuteCondition) Unmarshal(c *jsonkit.Codec, data []byte, p *workflow.ExecuteCondition) error {
	var dto workflowExecuteConditionDTO
	if err := json.Unmarshal(data, &dto); err != nil {
		return err
	}
	p.ID = workflow.ConditionID(dto.ID)
	p.Input = varNamesFromStrings(dto.Input)
	return nil
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
	Scope     VarScopeDTO        `json:"scope,omitempty"`
}

func (WorkflowEventDeclareVar) Marshal(c *jsonkit.Codec, v workflow.EventDeclareVar) ([]byte, error) {
	return json.Marshal(workflowEventDeclareVarDTO{
		EventID:   v.EventID,
		ProcessID: v.ProcessID,
		Timestamp: v.Timestamp,
		Path:      ToPathDTO(v.Path),
		Name:      string(v.Name),
		Scope:     ToVarScopeDTO(v.Scope),
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
	p.Scope = ToVarScope(dto.Scope)
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
	Value     any                `json:"value,omitempty"`
}

func (WorkflowEventSetVar) Marshal(c *jsonkit.Codec, v workflow.EventSetVar) ([]byte, error) {
	return json.Marshal(workflowEventSetVarDTO{
		EventID:   v.EventID,
		ProcessID: v.ProcessID,
		Timestamp: v.Timestamp,
		Path:      ToPathDTO(v.Path),
		Name:      string(v.Name),
		Value:     v.Value,
	})
}

func (WorkflowEventSetVar) Unmarshal(c *jsonkit.Codec, data []byte, p *workflow.EventSetVar) error {
	var dto workflowEventSetVarDTO
	if err := json.Unmarshal(data, &dto); err != nil {
		return err
	}
	p.EventID = dto.EventID
	p.ProcessID = dto.ProcessID
	p.Timestamp = dto.Timestamp
	p.Path = ToPath(dto.Path)
	p.Name = workflow.VarName(dto.Name)
	p.Value = dto.Value
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
	Input         []any              `json:"input,omitempty"`
	Output        []any              `json:"output,omitempty"`
	Definition    json.RawMessage    `json:"definition,omitempty"`
}

func (WorkflowEventParticipant) Marshal(c *jsonkit.Codec, v workflow.EventParticipant) ([]byte, error) {
	dto := workflowExecuteParticipantEventDTO{
		EventID:       v.EventID,
		ProcessID:     v.ProcessID,
		Timestamp:     v.Timestamp,
		ParticipantID: string(v.ParticipantID),
		Path:          ToPathDTO(v.Path),
		Input:         v.Input,
		Output:        v.Output,
	}
	if v.Definition != nil {
		b, err := c.Marshal(v.Definition)
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
	p.EventID = dto.EventID
	p.ProcessID = dto.ProcessID
	p.Timestamp = dto.Timestamp
	p.ParticipantID = workflow.ParticipantID(dto.ParticipantID)
	p.Path = ToPath(dto.Path)
	p.Input = dto.Input
	p.Output = dto.Output
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
	Input       []any              `json:"input"`
	Answer      bool               `json:"answer"`
	Timestamp   time.Time          `json:"timestamp"`
}

func (WorkflowEventCondition) Marshal(c *jsonkit.Codec, v workflow.EventCondition) ([]byte, error) {
	return json.Marshal(workflowExecuteConditionEventDTO{
		EventID:     v.EventID,
		ProcessID:   v.ProcessID,
		ConditionID: string(v.ConditionID),
		Path:        ToPathDTO(v.Path),
		Input:       v.Input,
		Answer:      v.Answer,
		Timestamp:   v.Timestamp,
	})
}

func (WorkflowEventCondition) Unmarshal(c *jsonkit.Codec, data []byte, p *workflow.EventCondition) error {
	var dto workflowExecuteConditionEventDTO
	if err := json.Unmarshal(data, &dto); err != nil {
		return err
	}
	p.EventID = dto.EventID
	p.ProcessID = dto.ProcessID
	p.ConditionID = workflow.ConditionID(dto.ConditionID)
	p.Path = ToPath(dto.Path)
	p.Input = dto.Input
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
	defBytes, err := c.Marshal(v.Definition)
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
}

func (WorkflowEventSpawn) Marshal(c *jsonkit.Codec, v workflow.EventSpawn) ([]byte, error) {
	return json.Marshal(workflowSpawnEventDTO{
		EventID:   v.EventID,
		ProcessID: v.ProcessID,
		ChildID:   v.ChildID,
		Name:      string(v.Name),
		Timestamp: v.Timestamp,
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

///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

// -- Schedule types (not Definitions/Events/Conditions) --

// Schedule

type WorkflowSchedule struct{}

var _ jsonkit.ITypeCodec[workflow.ProcessExecution] = WorkflowSchedule{}

type workflowScheduleDTO struct {
	ProcessID    workflow.ProcessID `json:"process_id"`
	StartTime    time.Time          `json:"start,omitzero"`
	CreatedAt    time.Time          `json:"created_at,omitzero"`
	FailureCount int                `json:"failure_count,omitzero"`
}

func (WorkflowSchedule) Marshal(c *jsonkit.Codec, v workflow.ProcessExecution) ([]byte, error) {
	return json.Marshal(workflowScheduleDTO{
		ProcessID:    v.ProcessID,
		StartTime:    v.StartTime,
		CreatedAt:    v.CreatedAt,
		FailureCount: v.FailureCount,
	})
}

func (WorkflowSchedule) Unmarshal(c *jsonkit.Codec, data []byte, p *workflow.ProcessExecution) error {
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

// ProcessChangeEvent is a polymorphic interface; each concrete implementation
// gets its own registered codec so jsonkit's @type envelope can dispatch
// Marshal/Unmarshal to the right DTO shape on the wire.

type WorkflowEventProcessSchedule struct{}

var _ jsonkit.ITypeCodec[workflow.ProcessSchedule] = WorkflowEventProcessSchedule{}

type workflowProcessScheduleDTO struct {
	ProcessID workflow.ProcessID `json:"process_id"`
}

func (WorkflowEventProcessSchedule) Marshal(c *jsonkit.Codec, v workflow.ProcessSchedule) ([]byte, error) {
	return json.Marshal(workflowProcessScheduleDTO{ProcessID: v.ProcessID})
}

func (WorkflowEventProcessSchedule) Unmarshal(c *jsonkit.Codec, data []byte, p *workflow.ProcessSchedule) error {
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
	return workflow.Path(dto)
}

// ToPathDTO converts a workflow.Path into its wire representation.
//
// A nil Path maps to a nil DTO, so that the zero value is omitted from the
// output instead of being rendered as an empty JSON array.
func ToPathDTO(p workflow.Path) PathDTO {
	if p == nil {
		return nil
	}
	return PathDTO(p)
}

// PathDTO is the wire representation of a workflow.Path.
//
// A path is a chain of scope names, so its wire form is a plain JSON array of
// strings — the same shape it had back when workflow.Path was a []string, which
// keeps histories recorded before the Scope detour readable without any shim.
type PathDTO []string

// ToVarScope converts the wire representation of a workflow.VarScope back into
// the domain type, following the same nil-preserving convention as ToPath.
func ToVarScope(dto VarScopeDTO) workflow.VarScope {
	if dto == nil {
		return nil
	}
	return workflow.VarScope(dto)
}

// ToVarScopeDTO converts a workflow.VarScope into its wire representation.
func ToVarScopeDTO(vs workflow.VarScope) VarScopeDTO {
	if vs == nil {
		return nil
	}
	return VarScopeDTO(vs)
}

// VarScopeDTO is the wire representation of a workflow.VarScope.
type VarScopeDTO []string

package wfencode

import (
	"encoding/json"
	wf "github.com/adamluzsi/frameless/pkg/workflow"
)

func MarshalJSON(v any) ([]byte, error) {
	return json.Marshal(v)
}

func UnmarshalJSON(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

type jsonEnvelope struct {
	Type string `json:"type"`
	jsonEnvelopeData
}

type jsonEnvelopeData any

func (e *jsonEnvelope) UnmarshalJSON(data []byte) error {
	type DTO struct {
		Type string
		Data json.RawMessage
	}
	var dto DTO
	if err := json.Unmarshal(data, &dto); err != nil {
		return err
	}
	switch dto.Type {
	case "pdef":
		var pdef wf.ProcessDefinition
		if err := json.Unmarshal(dto.Data, &pdef); err != nil {
			return err
		}
		e.any = pdef
	default:
		return json.Unmarshal(dto.Data, &e.Data)
	}
	return nil
}

func (e jsonEnvelope) MarshalJSON() ([]byte, error) {
	type DTO jsonEnvelope
	return json.Marshal(DTO(e))
}

func (e jsonEnvelope) VisitTask(visitor func(wf.Task)) {
	e.Data.(wf.Task).VisitTask(visitor)
}

func envelope(v any) any {
	switch v := v.(type) {
	case wf.ProcessDefinition:
		return jsonEnvelope{
			Type: "pdef",
			Data: wf.ProcessDefinition{
				ID:         v.ID,
				Task:       envelope(v.Task).(wf.Task),
				EntryPoint: envelope(v.EntryPoint),
			},
		}
	default:
		return v
	}
}

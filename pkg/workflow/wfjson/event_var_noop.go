package wfjson

import (
	"encoding/json"
	"time"

	"go.llib.dev/frameless/pkg/jsonkit"
	"go.llib.dev/frameless/pkg/workflow"
)

type WorkflowEventVarNoop struct{}

var _ jsonkit.ITypeCodec[workflow.EventVarNoop] = WorkflowEventVarNoop{}

type workflowEventVarNoopDTO struct {
	EventID   workflow.EventID   `json:"event_id"`
	ProcessID workflow.ProcessID `json:"process_id"`
	Timestamp time.Time          `json:"timestamp"`
	Path      PathDTO            `json:"path,omitempty"`
	Name      string             `json:"name"`
	Mutation  workflow.EventType `json:"mutation"`
}

func (WorkflowEventVarNoop) Marshal(_ *jsonkit.Codec, e workflow.EventVarNoop) ([]byte, error) {
	return json.Marshal(workflowEventVarNoopDTO{
		EventID: e.EventID, ProcessID: e.ProcessID, Timestamp: e.Timestamp,
		Path: ToPathDTO(e.Path), Name: string(e.Name), Mutation: e.Mutation,
	})
}

func (WorkflowEventVarNoop) Unmarshal(_ *jsonkit.Codec, data []byte, e *workflow.EventVarNoop) error {
	var dto workflowEventVarNoopDTO
	if err := json.Unmarshal(data, &dto); err != nil {
		return err
	}
	*e = workflow.EventVarNoop{
		EventID: dto.EventID, ProcessID: dto.ProcessID, Timestamp: dto.Timestamp,
		Path: ToPath(dto.Path), Name: workflow.VarName(dto.Name), Mutation: dto.Mutation,
	}
	return nil
}

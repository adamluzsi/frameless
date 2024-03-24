package health

import (
	"context"
	"fmt"
	"go.llib.dev/frameless/internal/consttypes"
	"go.llib.dev/frameless/pkg/dtos"
	"go.llib.dev/frameless/pkg/slicekit"
	"go.llib.dev/testcase/clock"
	"time"
)

type ReportJSONDTO struct {
	Status       string          `json:"status"`
	Name         string          `json:"name,omitempty"`
	Message      string          `json:"message,omitempty"`
	Issues       []IssueJSONDTO  `json:"issues,omitempty"`
	Dependencies []ReportJSONDTO `json:"dependencies,omitempty"`
	Timestamp    string          `json:"timestamp,omitempty"`
	Metrics      map[string]any  `json:"metrics,omitempty"`
}

var _ = dtos.Register[Report, ReportJSONDTO](
	func(ctx context.Context, ent Report) (ReportJSONDTO, error) {
		var metrics map[string]any
		metrics = ent.Metrics
		if len(metrics) == 0 { // TODO: cover this to prove that metrics are not in the json when they don't have values
			metrics = nil
		}
		return ReportJSONDTO{
			Status:  dtos.MustMap[string](ctx, ent.Status),
			Name:    ent.Name,
			Message: ent.Message,
			Issues: slicekit.Must(slicekit.Map[IssueJSONDTO](ent.Issues,
				func(v Issue) IssueJSONDTO {
					return dtos.MustMap[IssueJSONDTO](ctx, v)
				})),
			Dependencies: slicekit.Must(slicekit.Map[ReportJSONDTO](ent.Dependencies,
				func(v Report) ReportJSONDTO {
					return dtos.MustMap[ReportJSONDTO](ctx, v)
				})),
			Timestamp: ent.Timestamp.Format(time.RFC3339),
			Metrics:   metrics,
		}, nil
	},
	func(ctx context.Context, dto ReportJSONDTO) (Report, error) {
		timestamp := clock.TimeNow()
		if dto.Timestamp != "" {
			date, err := time.Parse(time.RFC3339, dto.Timestamp)
			if err != nil {
				return Report{}, fmt.Errorf("failed to parse timestamp: %w", err)
			}
			timestamp = date
		}
		hs := Report{
			Status:  dtos.MustMap[Status, string](ctx, dto.Status),
			Name:    dto.Name,
			Message: dto.Message,
			Issues: slicekit.Must(slicekit.Map[Issue](dto.Issues,
				func(v IssueJSONDTO) Issue {
					return dtos.MustMap[Issue](ctx, v)
				})),
			Dependencies: slicekit.Must(slicekit.Map[Report](dto.Dependencies,
				func(v ReportJSONDTO) Report {
					return dtos.MustMap[Report](ctx, v)
				})),
			Timestamp: timestamp,
			Metrics:   dto.Metrics,
		}
		return hs, hs.Validate()
	})

type IssueJSONDTO struct {
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

var _ = dtos.Register[Issue, IssueJSONDTO](
	func(ctx context.Context, ent Issue) (IssueJSONDTO, error) {
		return IssueJSONDTO{
			Code:    ent.Code.String(),
			Message: ent.Message,
		}, nil
	},
	func(ctx context.Context, dto IssueJSONDTO) (Issue, error) {
		return Issue{
			Code:    consttypes.String(dto.Code),
			Message: dto.Message,
		}, nil
	})

var _ = dtos.Register[Status, string](
	func(ctx context.Context, status Status) (string, error) {
		return status.String(), nil
	},
	func(ctx context.Context, s string) (Status, error) {
		status := Status(s)
		return status, status.Validate()
	})

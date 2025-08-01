package health

import (
	"context"
	"fmt"
	"time"

	"go.llib.dev/frameless/internal/constant"
	"go.llib.dev/frameless/pkg/dtokit"
	"go.llib.dev/frameless/pkg/slicekit"
	"go.llib.dev/testcase/clock"
)

type ReportJSONDTO struct {
	Status       string          `json:"status"`
	Name         string          `json:"name,omitempty"`
	Message      string          `json:"message,omitempty"`
	Issues       []IssueJSONDTO  `json:"issues,omitempty"`
	Dependencies []ReportJSONDTO `json:"dependencies,omitempty"`
	Timestamp    string          `json:"timestamp,omitempty"`
	Details      map[string]any  `json:"details,omitempty"`
}

var _ = dtokit.Register[Report, ReportJSONDTO](
	func(ctx context.Context, ent Report) (ReportJSONDTO, error) {
		var details map[string]any
		details = ent.Details
		if len(details) == 0 { // TODO: cover this to prove that metrics are not in the json when they don't have values
			details = nil
		}
		return ReportJSONDTO{
			Status:  dtokit.MustMap[string](ctx, ent.Status),
			Name:    ent.Name,
			Message: ent.Message,
			Issues: slicekit.Map[IssueJSONDTO](ent.Issues,
				func(v Issue) IssueJSONDTO {
					return dtokit.MustMap[IssueJSONDTO](ctx, v)
				}),
			Dependencies: slicekit.Map(ent.Dependencies,
				func(v Report) ReportJSONDTO {
					return dtokit.MustMap[ReportJSONDTO](ctx, v)
				}),
			Timestamp: ent.Timestamp.Format(time.RFC3339),
			Details:   details,
		}, nil
	},
	func(ctx context.Context, dto ReportJSONDTO) (Report, error) {
		timestamp := clock.Now()
		if dto.Timestamp != "" {
			date, err := time.Parse(time.RFC3339, dto.Timestamp)
			if err != nil {
				return Report{}, fmt.Errorf("failed to parse timestamp: %w", err)
			}
			timestamp = date
		}
		hs := Report{
			Status:  dtokit.MustMap[Status, string](ctx, dto.Status),
			Name:    dto.Name,
			Message: dto.Message,
			Issues: slicekit.Map(dto.Issues,
				func(v IssueJSONDTO) Issue {
					return dtokit.MustMap[Issue](ctx, v)
				}),
			Dependencies: slicekit.Map(dto.Dependencies,
				func(v ReportJSONDTO) Report {
					return dtokit.MustMap[Report](ctx, v)
				}),
			Timestamp: timestamp,
			Details:   dto.Details,
		}
		return hs, hs.Validate(ctx)
	})

type IssueJSONDTO struct {
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

var _ = dtokit.Register[Issue, IssueJSONDTO](
	func(ctx context.Context, ent Issue) (IssueJSONDTO, error) {
		return IssueJSONDTO{
			Code:    ent.Code.String(),
			Message: ent.Message,
		}, nil
	},
	func(ctx context.Context, dto IssueJSONDTO) (Issue, error) {
		return Issue{
			Code:    constant.String(dto.Code),
			Message: dto.Message,
		}, nil
	})

var _ = dtokit.Register[Status, string](
	func(ctx context.Context, status Status) (string, error) {
		return status.String(), nil
	},
	func(ctx context.Context, s string) (Status, error) {
		status := Status(s)
		return status, status.Validate(ctx)
	})

package health

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"go.llib.dev/frameless/internal/constant"
	"go.llib.dev/frameless/pkg/dtokit"
	"go.llib.dev/frameless/pkg/enum"
	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/iokit"
	"go.llib.dev/frameless/pkg/logger"
	"go.llib.dev/frameless/pkg/logging"
	"go.llib.dev/frameless/pkg/zerokit"
	"go.llib.dev/testcase/clock"
)

type Monitor struct {
	// ServiceName will be used to set the Report.Name field.
	ServiceName string
	// Checks contain the health checks about our own service.
	// Check should return with nil in case the check passed.
	// Check should return back with an Issue or a generic error, in case the check failed.
	// Returned generic errors are considered as an Issue with Down Status.
	Checks []Check
	// Dependencies represent our service's dependencies and their health state (Report).
	// DependencyCheck should come back always with a valid Report.
	Dependencies []DependencyCheck
	// Details represents our service's monitoring metrics.
	Details map[string]DetailCheck
}

type (
	// Check represents a health check.
	// Check supposed to yield back nil if the check passes.
	// Check should yield back an error in case the check detected a problem.
	// For problems, Check may return back an Issue to describe in detail the problem.
	// Most Errors will be considered as
	Check func(ctx context.Context) error
	// DependencyCheck serves as a health check for a specific dependency.
	// If an error occurs during the check,
	// it should be represented as an Issue in the returned Report.Issues list.
	//
	// For example, if a remote service is unreachable on the network,
	// it should be represented as an issue in the Report.Issues that the service is unreachable,
	// and the Issue.Causes should tell that this makes the given dependency health Status considered as Down.
	DependencyCheck func(ctx context.Context) Report
	// DetailCheck represents a metric reporting function. The result will be added to the Report.Metrics.
	// A DetailCheck results encompass analytical purpose, a status indicators for the service
	// for the given time when the service were called.
	// If numerical values are included, they should fluctuate over time, reflecting the current state.
	//
	// Values that behave differently depending on how long the application runs are not ideal.
	// For instance, a good metric value indicates the current throughput of the HTTP API,
	//
	// A challenging metric value would be a counter that counts the total handled requests number
	// from a given application's instance lifetime.
	DetailCheck func(ctx context.Context) (any, error)
)

func (m *Monitor) HealthCheck(ctx context.Context) Report {
	var report = Report{
		Name:    m.ServiceName,
		Details: make(map[string]any),
	}
	m.collectIssues(ctx, &report)
	m.collectDependencies(ctx, &report)
	m.collectDetails(ctx, &report)
	report.Correlate(ctx)
	return report
}

func (m *Monitor) collectIssues(ctx context.Context, hs *Report) {
	for _, checker := range m.Checks {
		err := checker(ctx)
		if err == nil {
			continue
		}
		var issue Issue
		if gotIssue, ok := errorkit.As[Issue](err); ok {
			issue = gotIssue
		} else {
			issue = Issue{
				Causes:  Down,
				Message: err.Error(),
			}
		}
		hs.Issues = append(hs.Issues, issue)
	}
}

func (m *Monitor) collectDependencies(ctx context.Context, hs *Report) {
	for _, checker := range m.Dependencies {
		dep := checker(ctx)
		dep.Correlate(ctx)
		hs.Dependencies = append(hs.Dependencies, dep)
	}
}

func (m *Monitor) HTTPHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var ctx = r.Context()

		report := m.HealthCheck(ctx)

		dto, err := dtokit.Map[ReportJSONDTO](ctx, report)
		if err != nil {
			logger.Error(ctx, "error mapping devops.HealthState to HealthState json DTO", logging.ErrField(err))
			return
		}

		statusCode, ok := healthStatusToHTTPStatus[report.Status]
		if !ok {
			if report.Status == Up {
				statusCode = http.StatusOK
			} else {
				statusCode = http.StatusServiceUnavailable
			}
		}
		w.WriteHeader(statusCode)

		// The JSON specification specifies that only space (ASCII decimal 32) character can be used for indentation.
		// To improve the health check endpoint readability with human consumption, two space indentations are used.
		data, err := json.MarshalIndent(dto, "", "  ")
		if err != nil {
			logger.Error(ctx, "error while marshaling health check DTO", logging.ErrField(err))
			return
		}
		_, _ = w.Write(data)
	})
}

func (m *Monitor) collectDetails(ctx context.Context, r *Report) {
	for name, check := range m.Details {
		val, err := check(ctx)
		if err != nil {
			r.Issues = append(r.Issues, Issue{
				Code:    "metric-error",
				Message: fmt.Sprintf("%q metric encountered an error.", name),
			})
			continue
		}
		r.Details[name] = val
	}
}

// TOFO: add more explanation to the fields

type Report struct {
	// Name field typically contains a descriptive name for the service or application.
	Name string
	// Status is the current health status of a given service.
	//
	// By default, an empty Status interpreted as Up Status.
	// If an Issue in Issues causes Status change, then it will be reflected in the Report.Status as well.
	// If a dependency has a non Up Status, then the current Status considered as PartialOutage.
	Status Status
	// Message field provides an explanation of the current state or specific issues (if any) affecting the service.
	// Message is optional, and when it's empty, the default is inferred from the Report.Status value.
	Message string
	// Issues is the list of issue that the health check functions were able to detect.
	// If an Issue in Report.Issues contain a Issue.Causes, then the Report.Status will be affected.
	Issues []Issue
	// Dependencies are the service dependencies, which are required for the service to function correctly.
	// If a Report has a problemating Status in Report.Dependencies, it will affect the Report.Status.
	Dependencies []Report
	// Timestamp represents the time at the health check report was created
	// Default is the current time in UTC.
	Timestamp time.Time
	// Details encompass analytical data and status indicators
	// for the service for the given time when the service were called.
	// For more about what values it should contain, read the documentation of Metric.
	Details map[string]any
}

func (r *Report) Validate(ctx context.Context) error {
	return r.Status.Validate(ctx)
}

func (r *Report) Correlate(ctx context.Context) {
	if r.Status.IsZero() {
		r.Status = Up
	}
	for _, issue := range r.Issues {
		if issue.Causes.IsZero() {
			continue
		}
		if issue.Causes.Validate(ctx) != nil {
			continue
		}
		if r.Status.IsLessSevere(issue.Causes) {
			r.Status = issue.Causes
		}
	}
	for i := range r.Dependencies {
		r.Dependencies[i].Correlate(ctx)
		dep := r.Dependencies[i]
		if r.Status == Up && dep.Status != Up {
			r.Status = PartialOutage
			break
		}
	}
	r.Message = zerokit.Coalesce(r.Message, StateMessage(r.Status))
	r.Timestamp = zerokit.Coalesce(r.Timestamp, clock.Now().UTC())
}

func (r Report) WithIssue(issue Issue) Report {
	r.Issues = append(append([]Issue{}, r.Issues...), issue)
	return r
}

// Issue represents an issue detected in during a health check.
type Issue struct {
	// Code is meant for programmatic processing of an issue detection.
	// Should contain no whitespace and use dash-case/snakecase/const-case.
	Code constant.String
	// Message can contain further details about the detected issue.
	Message string
	// Causes will indicate the status change this Issue will cause
	Causes Status
}

func (err Issue) Error() string {
	return fmt.Sprintf("code:%s\n%s", err.Code.String(), err.Message)
}

type Status string

const (
	// Up means that service is running correctly and able to respond to requests.
	Up Status = "UP"
	// Down means that service is not running or unresponsive.
	Down Status = "DOWN"
	// PartialOutage means that service is running, but one or more dependencies are experiencing issues.
	// PartialOutage also indicates that there has been a limited disruption or degradation in the service.
	// It typically affects only a subset of services or users, rather than the entire system.
	// Examples of partial outages include slower response times, intermittent errors,
	// or reduced functionality for specific features.
	PartialOutage Status = "PARTIAL_OUTAGE"
	// Degraded means that service is running but with reduced capabilities or performance.
	// When a system is in a Degraded state, it means that overall performance or functionality has deteriorated.
	// Unlike a PartialOutage, a Degraded state may impact a broader scope of services or users.
	// It could result in slower overall system performance, increased error rates, or reduced capacity.
	// Monitoring tools often detect this state based on predefined thresholds or deviations from expected behaviour.
	Degraded Status = "DEGRADED"
	// Maintenance means that service is currently undergoing maintenance or updates and might not function correctly.
	Maintenance Status = "MAINTENANCE"
	// Unknown means that service's status cannot be determined due to an error or lack of information.
	Unknown Status = "UNKNOWN"
)

var _ = enum.Register[Status](
	Up,
	Down,
	PartialOutage,
	Degraded,
	Maintenance,
	Unknown,
)

func (hss Status) Validate(context.Context) error {
	return enum.Validate[Status](hss)
}

const (
	minHealthStatusSeverity = 0
	maxHealthStatusSeverity = 10
)

var healthStatusSeverity = map[Status]int{
	Up:            minHealthStatusSeverity,
	Maintenance:   1,
	PartialOutage: 5,
	Degraded:      8,
	Unknown:       9,
	Down:          maxHealthStatusSeverity,
}

func (hss Status) IsLessSevere(oth Status) bool {
	severity, ok := healthStatusSeverity[hss]
	if !ok {
		severity = maxHealthStatusSeverity
	}
	othSeverity, ok := healthStatusSeverity[oth]
	if !ok {
		severity = maxHealthStatusSeverity
	}
	return severity < othSeverity
}

func (hss Status) String() string {
	return string(hss)
}

func (hss Status) IsZero() bool {
	var zero Status
	return hss == zero
}

var mapHealthStateMessage = map[Status]string{
	Up:            "The service is running correctly and able to respond to requests.",
	Down:          "The service is not running or unresponsive.",
	PartialOutage: "The service is running, but one or more dependencies are experiencing issues.",
	Degraded:      "The service is running but with reduced capabilities or performance.",
	Maintenance:   "The service is currently undergoing maintenance or updates and might not function correctly.",
	Unknown:       "The service's status cannot be determined due to an error or lack of information.",
}

func StateMessage(s Status) string {
	msg, ok := mapHealthStateMessage[s]
	if !ok {
		return fmt.Sprintf("no health state message available for: %s", s.String())
	}
	return msg
}

var healthStatusToHTTPStatus = map[Status]int{
	Up:            http.StatusOK,
	Down:          http.StatusServiceUnavailable,
	PartialOutage: http.StatusServiceUnavailable,
	Degraded:      http.StatusServiceUnavailable,
	Maintenance:   http.StatusServiceUnavailable,
	Unknown:       http.StatusInternalServerError,
}

var _ = func() struct{} {
	for _, s := range enum.Values[Status]() {
		if _, ok := healthStatusToHTTPStatus[s]; !ok {
			panic(fmt.Errorf("implementation issue, %s is not mapped to an HTTP StatusCode", s.String()))
		}
	}
	return struct{}{}
}()

type HTTPHealthCheckConfig struct {
	Name          string
	HTTPClient    *http.Client
	BodyReadLimit iokit.ByteSize
	Unmarshal     func(ctx context.Context, data []byte, ptr *Report) error
}

func HTTPHealthCheck(healthCheckEndpointURL string, config *HTTPHealthCheckConfig) func(ctx context.Context) Report {
	c := zerokit.Coalesce(config, &HTTPHealthCheckConfig{})
	client := zerokit.Coalesce(c.HTTPClient, http.DefaultClient)
	unmarshal := zerokit.Coalesce(c.Unmarshal, defaultHealthResponseUnmarshal)
	defaultName := zerokit.Coalesce(c.Name, healthCheckEndpointURL)
	readLimit := zerokit.Coalesce(c.BodyReadLimit, 25*iokit.Megabyte)
	return func(ctx context.Context) (s Report) {
		s = Report{Name: defaultName}
		defer s.Correlate(ctx)
		defer func() { s.Name = zerokit.Coalesce(s.Name, defaultName) }()

		resp, err := client.Get(healthCheckEndpointURL)
		if err != nil {
			s.Issues = append(s.Issues, Issue{
				Code:    "network-error",
				Message: err.Error(),
				Causes:  Down,
			})
			return s
		}

		defer func() { s.Status = zerokit.Coalesce(s.Status, statusFromHTTPStatusCode(resp.StatusCode)) }()

		data, err := iokit.ReadAllWithLimit(resp.Body, readLimit)
		if err != nil {
			s.Issues = append(s.Issues, Issue{
				Code: "too-large-health-response-received",
				Message: fmt.Sprintf("The received response is larger than the configured %s",
					iokit.FormatByteSize(readLimit)),
				Causes: Down,
			})
			return s
		}

		if 0 < len(data) {
			// By default we infer the state from the status code,
			// but if we have response data, we can use that.
			if err := unmarshal(ctx, data, &s); err != nil {
				s.Issues = append(s.Issues, Issue{
					Code:    "malformed-health-check-response-body",
					Message: err.Error(),
					Causes:  Unknown,
				})
				return s
			}
		}

		return s
	}
}

func defaultHealthResponseUnmarshal(ctx context.Context, data []byte, ptr *Report) error {
	var dto ReportJSONDTO
	if err := json.Unmarshal(data, &dto); err != nil {
		return err
	}
	ent, err := dtokit.Map[Report, ReportJSONDTO](ctx, dto)
	if err != nil {
		return err
	}
	*ptr = ent
	return nil
}

// statusFromHTTPStatusCode will evaluate an HTTP status code received from a /health endpoint,
// and returns back the Status it represents.
// It follows the Kubernetes way of interpreting the http status code.
//
//   - 200-299: The service is healthy and ready to accept traffic.
//
//   - 4xx: The request was invalid or malformed, indicating an issue with the client.
//     For example, this could indicate that the client sent a bad request or provided invalid parameters.
//
//   - 5xx: There was a server-side error.
//     This indicates that there is something wrong on the server side,
//     and the service is not able to handle the request.
//
//   - 404: The health endpoint was not found at the specified location.
//     This indicates that there may be a problem with the service's configuration or deployment.//
//
//   - 503 (Service Unavailable): The service is currently unavailable,
//     but it is expected to become available again in the future.
//     Kubernetes will continue to check the service until it starts responding with a different status code.
func statusFromHTTPStatusCode(code int) Status {
	switch {
	case 200 <= code && code <= 299:
		return Up
	case code == http.StatusNotFound:
		return Up
	case 500 <= code && code <= 599:
		return Down
	default:
		return Unknown
	}
}

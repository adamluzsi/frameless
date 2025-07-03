package health_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"go.llib.dev/frameless/pkg/devops/health"
	"go.llib.dev/frameless/pkg/dtokit"
	"go.llib.dev/frameless/pkg/enum"
	"go.llib.dev/frameless/pkg/httpkit"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/clock/timecop"
	"go.llib.dev/testcase/let"
	"go.llib.dev/testcase/random"
)

var rnd = random.New(random.CryptoSeed{})

func ExampleMonitor_HTTPHandler() {
	var m = health.Monitor{
		Checks: []health.Check{
			func(ctx context.Context) error {
				return nil // all good
			},
		},
	}

	mux := http.NewServeMux()
	mux.Handle("/health", m.HTTPHandler())
	_ = http.ListenAndServe("0.0.0.0:8080", mux)
}

func ExampleMonitor_check() {
	const detailKeyForHTTPRetryPerSec = "http-retry-average-per-second"
	appDetails := sync.Map{}

	var hm = health.Monitor{
		Checks: []health.Check{
			func(ctx context.Context) error {
				value, ok := appDetails.Load(detailKeyForHTTPRetryPerSec)
				if !ok {
					return nil
				}
				averagePerSec, ok := value.(int)
				if !ok {
					return nil
				}
				if 42 < averagePerSec {
					return health.Issue{
						Causes: health.Degraded,
						Code:   "too-many-http-request-retries",
						Message: "There could be an underlying networking issue, " +
							"that needs to be looked into, the system is working, " +
							"but the retry attemt average shouldn't be so high",
					}
				}
				return nil
			},
		},
	}

	ctx := context.Background()
	hs := hm.HealthCheck(ctx)
	_ = hs // use the results
}

func ExampleMonitor_dependency() {
	var hm health.Monitor
	var db *sql.DB // populate it with a live db connection

	hm.Dependencies = append(hm.Dependencies, func(ctx context.Context) health.Report {
		var hs health.Report
		err := db.PingContext(ctx)

		if err != nil {
			hs.Issues = append(hs.Issues, health.Issue{
				Causes:  health.Down,
				Code:    "xy-db-disconnected",
				Message: "failed to ping the database through the connection",
			})
		}

		// additional health checks on the DB dependency

		return hs
	})

	ctx := context.Background()
	hs := hm.HealthCheck(ctx)
	_ = hs // use the results
}

func TestMonitor_HealthCheck(t *testing.T) {
	var ServiceName = rnd.StringNC(5, random.CharsetAlpha())

	t.Run("no-checks-or-dependencies", func(t *testing.T) {
		hc := health.Monitor{ServiceName: ServiceName}

		state := hc.HealthCheck(context.Background())
		assert.Equal(t, health.Up, state.Status)
		assert.Equal(t, hc.ServiceName, state.Name)
		assert.NotNil(t, state.Details)
	})

	t.Run("all-checks-pass", func(t *testing.T) {
		hc := health.Monitor{}

		var gotCount int
		expCount := rnd.Repeat(1, 5, func() {
			hc.Checks = append(hc.Checks, func(ctx context.Context) error {
				gotCount++
				return nil
			})
		})

		state := hc.HealthCheck(context.Background())
		assert.Equal(t, health.Up, state.Status)
		assert.Empty(t, state.Issues)
		assert.Equal(t, gotCount, expCount)
	})

	t.Run("one-check-fails", func(t *testing.T) {
		hc := health.Monitor{}

		hc.Checks = append(hc.Checks, func(ctx context.Context) error {
			return nil
		})
		hc.Checks = append(hc.Checks, func(ctx context.Context) error {
			return health.Issue{Causes: health.Down}
		})

		state := hc.HealthCheck(context.Background())
		assert.Equal(t, health.Down, state.Status)
	})

	t.Run("health-check-fails-with-generic-error", func(t *testing.T) {
		hc := health.Monitor{}

		hc.Checks = append(hc.Checks, func(ctx context.Context) error {
			return nil
		})
		hc.Checks = append(hc.Checks, func(ctx context.Context) error {
			return errors.New("boom")
		})

		state := hc.HealthCheck(context.Background())
		assert.Equal(t, health.Down, state.Status)
	})

	t.Run("check reports an issue, but the health state is not degraded", func(t *testing.T) {
		hc := health.Monitor{}

		expIssue := health.Issue{Causes: health.Up}

		hc.Checks = append(hc.Checks, func(ctx context.Context) error {
			return expIssue
		})

		state := hc.HealthCheck(context.Background())
		assert.Equal(t, health.Up, state.Status)
		assert.Contain(t, state.Issues, expIssue)
	})

	t.Run("all-dependencies-pass", func(t *testing.T) {
		hc := health.Monitor{}

		hc.Dependencies = append(hc.Dependencies, func(ctx context.Context) health.Report {
			return health.Report{Status: health.Up}
		})

		state := hc.HealthCheck(context.Background())
		assert.Equal(t, health.Up, state.Status)
	})

	t.Run("one-dependency-fails", func(t *testing.T) {
		hc := health.Monitor{}

		hc.Dependencies = append(hc.Dependencies, func(ctx context.Context) health.Report {
			return health.Report{Status: health.Up}
		})
		hc.Dependencies = append(hc.Dependencies, func(ctx context.Context) health.Report {
			return health.Report{Status: health.Down}
		})

		state := hc.HealthCheck(context.Background())
		assert.Equal(t, health.PartialOutage, state.Status)
	})

	t.Run("health state message contains the human readable state message", func(t *testing.T) {
		hc := health.Monitor{}

		var status health.Status
		hc.Checks = append(hc.Checks, func(ctx context.Context) error {
			return health.Issue{Causes: status}
		})

		ctx := context.Background()
		for _, hsv := range enum.Values[health.Status]() {
			status = hsv
			healthState := hc.HealthCheck(ctx)
			assert.Equal(t, healthState.Message, health.StateMessage(status))
		}
	})

	t.Run("when dependency health state message is provided, it is kept as is", func(t *testing.T) {
		hc := health.Monitor{}

		var (
			status  health.Status
			message = rnd.Error().Error()
		)
		hc.Dependencies = append(hc.Dependencies, func(ctx context.Context) health.Report {
			return health.Report{Status: status, Message: message}
		})

		ctx := context.Background()
		for _, hsv := range enum.Values[health.Status]() {
			status = hsv
			healthState := hc.HealthCheck(ctx)
			assert.OneOf(t, healthState.Dependencies, func(it testing.TB, got health.Report) {
				assert.Equal(it, got.Status, status)
				assert.Equal(it, got.Message, message)
			})
		}
	})

	t.Run("dependency health state message is populated when it is empty", func(t *testing.T) {
		hc := health.Monitor{}

		var status health.Status
		hc.Dependencies = append(hc.Dependencies, func(ctx context.Context) health.Report {
			return health.Report{Status: status, Message: ""}
		})

		ctx := context.Background()
		for _, hsv := range enum.Values[health.Status]() {
			status = hsv
			healthState := hc.HealthCheck(ctx)
			assert.OneOf(t, healthState.Dependencies, func(it testing.TB, got health.Report) {
				assert.Equal(it, got.Status, status)
				assert.Equal(it, got.Message, health.StateMessage(got.Status))
			})
		}
	})

	t.Run("dependency health state message is kept when populated", func(t *testing.T) {
		monitor := health.Monitor{}

		monitor.Dependencies = append(monitor.Dependencies, func(ctx context.Context) health.Report {
			return health.Report{
				Name: "downstream-service-42",
				Dependencies: []health.Report{
					{Name: "downstream-service-128"},
				},
			}
		})

		ctx := context.Background()

		report := monitor.HealthCheck(ctx)

		assert.NotEmpty(t, report.Dependencies)
		assert.OneOf(t, report.Dependencies, func(t testing.TB, dep health.Report) {
			assert.OneOf(t, dep.Dependencies, func(t testing.TB, depdep health.Report) {
				assert.Equal(t, depdep.Status, health.Up)
				assert.NotEmpty(t, depdep.Message)
			})
		})
	})

	t.Run("the dependency's dependencies report is correlated", func(t *testing.T) {
		hc := health.Monitor{}

		var status health.Status
		hc.Dependencies = append(hc.Dependencies, func(ctx context.Context) health.Report {
			return health.Report{Status: status, Message: "foo"}
		})

		ctx := context.Background()
		for _, hsv := range enum.Values[health.Status]() {
			status = hsv
			healthState := hc.HealthCheck(ctx)
			assert.OneOf(t, healthState.Dependencies, func(it testing.TB, got health.Report) {
				assert.Equal(it, got.Status, status)
				assert.Equal(it, got.Message, "foo")
			})
		}
	})

	t.Run("dependency timestamp message is populated when it is empty", func(t *testing.T) {
		hc := health.Monitor{
			Dependencies: []health.DependencyCheck{
				func(ctx context.Context) health.Report {
					return health.Report{
						Name: "the-name",
					}
				},
			},
		}

		report := hc.HealthCheck(context.Background())
		assert.NotEmpty(t, report.Timestamp)
		assert.NotEmpty(t, report.Dependencies)
		for _, dep := range report.Dependencies {
			assert.NotEmpty(t, dep.Timestamp)
			assert.NotEmpty(t, dep.Name)
			assert.NotEmpty(t, dep.Status)
			assert.NotEmpty(t, dep.Message)
		}
	})

	t.Run("dependency health state status is not set, but has issues, then the highest severity level status is used", func(t *testing.T) {
		hc := health.Monitor{}

		hc.Dependencies = append(hc.Dependencies, func(ctx context.Context) health.Report {
			return health.Report{
				Issues: []health.Issue{
					{Causes: health.Up},
					{Causes: health.PartialOutage},
					{Causes: health.Down},
					{Causes: health.Degraded},
				},
			}
		})

		ctx := context.Background()
		hs := hc.HealthCheck(ctx)
		assert.NotEmpty(t, hs.Dependencies)
		assert.OneOf(t, hs.Dependencies, func(it testing.TB, got health.Report) {
			assert.Equal(t, health.Down, got.Status)
			assert.Equal(t, health.StateMessage(health.Down), got.Message)
		})

	})

	t.Run("metrics are set during the health check evaluation process", func(t *testing.T) {
		m := health.Monitor{
			Details: map[string]health.DetailCheck{
				"x-metric": func(ctx context.Context) (val any, err error) {
					return 42, nil
				},
			},
		}

		report := m.HealthCheck(context.Background())
		assert.Equal(t, health.Up, report.Status)
		assert.Empty(t, report.Issues)
		assert.NotEmpty(t, report.Details)
		assert.Equal[any](t, report.Details["x-metric"], 42)
	})

	t.Run("an error during a metric evaluation is reported as a non fatal issue", func(t *testing.T) {
		m := health.Monitor{
			Details: map[string]health.DetailCheck{
				"x-metric": func(ctx context.Context) (val any, err error) {
					return 42, fmt.Errorf("boom")
				},
			},
		}

		report := m.HealthCheck(context.Background())

		assert.Equal(t, health.Up, report.Status,
			"a metric error should not cause an issue in the service health")

		assert.OneOf(t, report.Issues, func(t testing.TB, got health.Issue) {
			assert.Equal(t, got.Code, "metric-error")
			assert.Contain(t, got.Message, "x-metric")
			assert.Contain(t, got.Message, "error")
		})
	})

	t.Run("contains a timestamp", func(t *testing.T) {
		m := health.Monitor{}
		now := time.Now()
		timecop.Travel(t, now, timecop.Freeze)
		report := m.HealthCheck(context.Background())
		assert.Equal(t, now.UTC(), report.Timestamp)
	})
}

func TestMonitor_race(t *testing.T) {
	m := &health.Monitor{
		ServiceName: "name",
		Checks: []health.Check{
			func(ctx context.Context) error {
				return nil
			},
		},
		Dependencies: []health.DependencyCheck{
			func(ctx context.Context) health.Report {
				return health.Report{
					Status: health.Up,
					Name:   "dep",
				}
			},
		},
		Details: map[string]health.DetailCheck{
			"detail-name": func(ctx context.Context) (any, error) {
				return 42, nil
			},
		},
	}

	ctx := context.Background()
	testcase.Race(
		func() { m.HealthCheck(ctx) },
		func() { m.HealthCheck(ctx) },
		func() { m.HealthCheck(ctx) },
	)

	h := m.HTTPHandler()

	testcase.Race(
		func() { h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil)) },
		func() { h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil)) },
		func() { h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil)) },
	)
}

func TestMonitor_HTTPHandler(t *testing.T) {
	s := testcase.NewSpec(t)

	subject := testcase.Let(s, func(t *testcase.T) *health.Monitor {
		return &health.Monitor{}
	})

	act := func(t *testcase.T) *httptest.ResponseRecorder {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		subject.Get(t).HTTPHandler().ServeHTTP(rr, req)
		return rr
	}

	now := testcase.Let(s, func(t *testcase.T) time.Time {
		return time.Now().UTC()
	})
	s.Before(func(t *testcase.T) {
		timecop.Travel(t, now.Get(t), timecop.Freeze)
	})

	s.Then("we get back a health response report", func(t *testcase.T) {
		resp := act(t)
		t.Must.Equal(http.StatusOK, resp.Code)
		var dto health.ReportJSONDTO
		t.Must.NoError(json.Unmarshal(resp.Body.Bytes(), &dto))
		t.Must.Equal(dto.Status, health.Up.String())
		t.Must.Equal(dto.Message, health.StateMessage(health.Up))
		t.Must.Empty(dto.Issues)
		t.Must.Equal(dto.Timestamp, now.Get(t).Format(time.RFC3339))
	})

	s.When("dependency is registered", func(s *testcase.Spec) {
		depState := testcase.Let[health.Report](s, func(t *testcase.T) health.Report {
			return health.Report{
				Status:  health.Up,
				Name:    "the name",
				Message: "the message",
			}
		})
		s.Before(func(t *testcase.T) {
			subject.Get(t).Dependencies = append(subject.Get(t).Dependencies, func(ctx context.Context) health.Report {
				return depState.Get(t)
			})
		})

		s.Then("the dependency health state is returned back", func(t *testcase.T) {
			resp := act(t)
			t.Must.Equal(http.StatusOK, resp.Code)
			var dto health.ReportJSONDTO
			t.Must.NoError(json.Unmarshal(resp.Body.Bytes(), &dto))
			t.Must.Equal(dto.Status, health.Up.String())
			t.Must.Equal(dto.Message, health.StateMessage(health.Up))
			t.Must.Empty(dto.Issues)
			t.Must.NotEmpty(dto.Dependencies)
			assert.OneOf(t, dto.Dependencies, func(it testing.TB, got health.ReportJSONDTO) {
				assert.Equal(it, got.Status, depState.Get(t).Status.String())
				assert.Equal(it, got.Name, depState.Get(t).Name)
				assert.Equal(it, got.Message, depState.Get(t).Message)
			})
		})

		s.And("it has an issue", func(s *testcase.Spec) {
			depState.Let(s, func(t *testcase.T) health.Report {
				return health.Report{
					Status:  health.Down,
					Name:    "the name",
					Message: "the message",
				}
			})

			s.Then("the dependency's health issue is reflected on the response", func(t *testcase.T) {
				resp := act(t)
				t.Must.Equal(http.StatusServiceUnavailable, resp.Code)
				var dto health.ReportJSONDTO
				t.Must.NoError(json.Unmarshal(resp.Body.Bytes(), &dto))
				t.Must.Equal(dto.Status, health.PartialOutage.String())
				t.Must.Equal(dto.Message, health.StateMessage(health.PartialOutage))
				t.Must.Empty(dto.Issues)
				t.Must.NotEmpty(dto.Dependencies)
				assert.OneOf(t, dto.Dependencies, func(it testing.TB, got health.ReportJSONDTO) {
					assert.Equal(it, got.Status, depState.Get(t).Status.String())
					assert.Equal(it, got.Name, depState.Get(t).Name)
					assert.Equal(it, got.Message, depState.Get(t).Message)
				})
			})

		})
	})

	s.When("detail is registered", func(s *testcase.Spec) {
		detailVal := let.IntB(s, 0, 100)

		s.Before(func(t *testcase.T) {
			subject.Get(t).Details = map[string]health.DetailCheck{
				"x-detail": func(ctx context.Context) (any, error) {
					return detailVal.Get(t), nil
				},
			}
		})

		s.Then("the detail result is returned back", func(t *testcase.T) {
			resp := act(t)
			t.Must.Equal(http.StatusOK, resp.Code)
			var dto health.ReportJSONDTO
			t.Must.NoError(json.Unmarshal(resp.Body.Bytes(), &dto))
			t.Must.NotEmpty(dto.Details)
			t.Must.Equal(dto.Details["x-detail"], float64(detailVal.Get(t)))
		})
	})

}

func TestIssue(t *testing.T) {
	i := health.Issue{
		Code:    "error-code",
		Message: rnd.String(),
		Causes:  "status",
	}
	var _ error = i // interface check
	assert.NotEmpty(t, i.Error())
	assert.Contain(t, i.Error(), i.Code)
	assert.Contain(t, i.Error(), i.Message)
}

func ExampleHTTPHealthCheck() {
	var m = health.Monitor{
		Dependencies: []health.DependencyCheck{
			health.HTTPHealthCheck("https://www.example.com/health", nil),
		},
	}

	_ = m
}

func TestHTTPHealthCheck(t *testing.T) {
	s := testcase.NewSpec(t)

	var monitor = testcase.Let(s, func(t *testcase.T) *health.Monitor {
		return &health.Monitor{}
	})

	var (
		healthCheckEndpoint = testcase.Let[http.Handler](s, nil)
		remoteService       = testcase.Let(s, func(t *testcase.T) *httptest.Server {
			mux := http.NewServeMux()
			mux.Handle("/health", healthCheckEndpoint.Get(t))
			srv := httptest.NewServer(mux)
			t.Defer(srv.Close)
			return srv
		})
	)

	var (
		ctx = let.Context(s)
	)
	act := func(t *testcase.T) health.Report {
		return monitor.Get(t).HealthCheck(ctx.Get(t))
	}

	s.When("service uses frameless health check http handler", func(s *testcase.Spec) {
		othMonitor := testcase.Let(s, func(t *testcase.T) *health.Monitor {
			return &health.Monitor{ServiceName: "TheServiceName"}
		})
		healthCheckEndpoint.Let(s, func(t *testcase.T) http.Handler {
			return othMonitor.Get(t).HTTPHandler()
		})
		s.Before(func(t *testcase.T) {
			monitor.Get(t).Dependencies = append(monitor.Get(t).Dependencies, health.HTTPHealthCheck(remoteService.Get(t).URL+"/health", nil))
		})

		s.Then("dependency is reported", func(t *testcase.T) {
			report := act(t)
			assert.NotEmpty(t, report)
			assert.Equal(t, health.Up, report.Status)
			assert.NotEmpty(t, report.Dependencies)
			assert.OneOf(t, report.Dependencies, func(it testing.TB, got health.Report) {
				assert.Equal(it, got.Name, othMonitor.Get(t).ServiceName)
				assert.Equal(it, got.Status, health.Up)
			})
		})

		s.And("if the downstream service has an issue", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				othMonitor.Get(t).Checks = append(othMonitor.Get(t).Checks, func(ctx context.Context) error {
					return health.Issue{
						Code:    "error-for-test",
						Message: "boom",
						Causes:  health.Down,
					}
				})
			})

			s.Then("the dependency's issue is reported", func(t *testcase.T) {
				report := act(t)
				assert.NotEmpty(t, report)
				assert.Equal(t, health.PartialOutage, report.Status)
				assert.NotEmpty(t, report.Dependencies)
				assert.OneOf(t, report.Dependencies, func(it testing.TB, got health.Report) {
					assert.Equal(it, got.Name, othMonitor.Get(t).ServiceName)
					assert.Equal(it, got.Status, health.Down)
				})
			})
		})

		s.And("downstream service has its own dependencies", func(s *testcase.Spec) {
			nameOfTheDependencyOfOurDependency := testcase.Let(s, func(t *testcase.T) string {
				return t.Random.StringNC(5, random.CharsetAlpha())
			})

			s.Before(func(t *testcase.T) {
				othMonitor.Get(t).Dependencies = append(othMonitor.Get(t).Dependencies, func(ctx context.Context) health.Report {
					return health.Report{
						Status: health.Up,
						Name:   nameOfTheDependencyOfOurDependency.Get(t),
					}
				})
			})

			s.Then("the dependency of our dependendency is also reported", func(t *testcase.T) {
				report := act(t)
				assert.NotEmpty(t, report)
				assert.Equal(t, health.Up, report.Status)
				assert.NotEmpty(t, report.Dependencies)
				assert.OneOf(t, report.Dependencies, func(it testing.TB, got health.Report) {
					assert.NotEmpty(it, got.Dependencies)
					assert.OneOf(it, got.Dependencies, func(it testing.TB, got health.Report) {
						assert.Equal(it, got.Name, nameOfTheDependencyOfOurDependency.Get(t))
					})
				})
			})
		})
	})

	s.When("the health check endpoint is misconfigured", func(s *testcase.Spec) {
		healthCheckEndpoint.Let(s, func(t *testcase.T) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
		})
		monitor.Let(s, func(t *testcase.T) *health.Monitor {
			m := monitor.Super(t)
			m.Dependencies = append(m.Dependencies,
				health.HTTPHealthCheck(remoteService.Get(t).URL+"/incorrect/health/check/endpoint", nil))
			return m
		})

		s.Then("our service marked as unhealty", func(t *testcase.T) {
			report := act(t)
			assert.NotEqual(t, report.Status, health.Up)
		})

		s.Then("the dependency state marked as UNKNOWN", func(t *testcase.T) {
			report := act(t)
			assert.OneOf(t, report.Dependencies, func(it testing.TB, got health.Report) {
				assert.Equal(it, got.Status, health.Unknown)
			})
		})
	})

	s.When("health check endpoint only uses status codes", func(s *testcase.Spec) {
		statusCode := testcase.LetValue[int](s, http.StatusOK)
		healthCheckEndpoint.Let(s, func(t *testcase.T) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(statusCode.Get(t))
			})
		})
		monitor.Let(s, func(t *testcase.T) *health.Monitor {
			m := monitor.Super(t)
			m.Dependencies = append(m.Dependencies,
				health.HTTPHealthCheck(remoteService.Get(t).URL+"/health", nil))
			return m
		})

		s.Then("our service reported as UP", func(t *testcase.T) {
			report := act(t)
			assert.NotEmpty(t, report)
			assert.Equal(t, health.Up, report.Status)
		})

		s.Then("it reports it as a dependency", func(t *testcase.T) {
			report := act(t)
			assert.OneOf(t, report.Dependencies, func(it testing.TB, got health.Report) {
				assert.Contain(it, got.Name, remoteService.Get(t).URL)
				assert.Equal(it, got.Status, health.Up)
			})
		})

		s.And("the /health endpoint suggest an outage", func(s *testcase.Spec) {
			statusCode.LetValue(s, http.StatusServiceUnavailable)

			s.Then("our service is affected", func(t *testcase.T) {
				report := act(t)
				assert.NotEmpty(t, report)
				assert.Equal(t, report.Status, health.PartialOutage)
			})

			s.Then("the dependency marked as down", func(t *testcase.T) {
				report := act(t)
				assert.OneOf(t, report.Dependencies, func(it testing.TB, got health.Report) {
					assert.Contain(it, got.Name, remoteService.Get(t).URL)
					assert.Equal(it, got.Status, health.Down)
				})
			})
		})
	})

	s.When("health check endpoint uses its own format", func(s *testcase.Spec) {
		type HealthReportFormat struct {
			Status string `enum:"healthy,unhealthy," json:"status"`
		}

		status := testcase.LetValue[string](s, "healthy")
		healthCheckEndpoint.Let(s, func(t *testcase.T) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(HealthReportFormat{
					Status: status.Get(t),
				})
			})
		})
		monitor.Let(s, func(t *testcase.T) *health.Monitor {
			m := monitor.Super(t)
			m.Dependencies = append(m.Dependencies,
				health.HTTPHealthCheck(remoteService.Get(t).URL+"/health", &health.HTTPHealthCheckConfig{
					Name: "service-x",
					Unmarshal: func(ctx context.Context, data []byte, ptr *health.Report) error {
						// Mapping
						var dto HealthReportFormat
						if err := json.Unmarshal(data, &dto); err != nil {
							return err
						}
						switch {
						case dto.Status == "healthy":
							ptr.Status = health.Up
						case dto.Status == "unhealthy":
							ptr.Status = health.Down
						default:
							ptr.Status = health.Unknown
						}
						return nil
					},
				}))
			return m
		})

		s.Then("our service reported as UP", func(t *testcase.T) {
			report := act(t)
			assert.NotEmpty(t, report)
			assert.Equal(t, health.Up, report.Status)
		})

		s.Then("it reports it as a dependency", func(t *testcase.T) {
			report := act(t)
			assert.OneOf(t, report.Dependencies, func(it testing.TB, got health.Report) {
				assert.Equal(it, got.Name, "service-x")
				assert.Equal(it, got.Status, health.Up)
			})
		})

		s.And("the /health endpoint suggest an outage", func(s *testcase.Spec) {
			status.LetValue(s, "unhealthy")

			s.Then("our service is affected", func(t *testcase.T) {
				report := act(t)
				assert.NotEmpty(t, report)
				assert.Equal(t, report.Status, health.PartialOutage)
			})

			s.Then("the dependency marked as down", func(t *testcase.T) {
				report := act(t)
				assert.OneOf(t, report.Dependencies, func(it testing.TB, got health.Report) {
					assert.Equal(it, got.Name, "service-x")
					assert.Equal(it, got.Status, health.Down)
				})
			})
		})
	})

	s.When("authentication can be done by injectin the HTTPClient", func(s *testcase.Spec) {
		healthCheckEndpoint.Let(s, func(t *testcase.T) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") == "" {
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				w.WriteHeader(http.StatusOK)
			})
		})
		monitor.Let(s, func(t *testcase.T) *health.Monitor {
			m := monitor.Super(t)
			m.Dependencies = append(m.Dependencies,
				health.HTTPHealthCheck(remoteService.Get(t).URL+"/health", &health.HTTPHealthCheckConfig{
					Name: "service-x",
					HTTPClient: &http.Client{
						Transport: httpkit.RoundTripperFunc(func(request *http.Request) (*http.Response, error) {
							request.Header.Set("Authorization", "yay")
							return http.DefaultTransport.RoundTrip(request)
						}),
					},
				}))
			return m
		})

		s.Then("our service reported as UP", func(t *testcase.T) {
			report := act(t)
			assert.NotEmpty(t, report)
			assert.Equal(t, health.Up, report.Status)
		})

		s.Then("it reports it as a dependency", func(t *testcase.T) {
			report := act(t)
			assert.OneOf(t, report.Dependencies, func(it testing.TB, got health.Report) {
				assert.Equal(it, got.Name, "service-x")
				assert.Equal(it, got.Status, health.Up)
			})
		})
	})

	// TODO:
	//  - authentication
}

func TestExampleResponse(t *testing.T) {
	if _, ok := os.LookupEnv("example"); !ok {
		t.Skip()
	}

	const detailKeyForHTTPRetryPerSec = "http-retry-average-per-second"
	var appDetails sync.Map
	appDetails.Store(detailKeyForHTTPRetryPerSec, 42)

	m := health.Monitor{
		// our service related checks
		Checks: []health.Check{
			func(ctx context.Context) error {
				value, ok := appDetails.Load(detailKeyForHTTPRetryPerSec)
				if !ok {
					return nil
				}
				averagePerSec, ok := value.(int)
				if !ok {
					return nil
				}
				if 42 < averagePerSec {
					return health.Issue{
						Causes: health.Degraded,
						Code:   "too-many-http-request-retries",
						Message: "There could be an underlying networking issue, " +
							"that needs to be looked into, the system is working, " +
							"but the retry attemt average shouldn't be so high",
					}
				}
				return nil
			},
		},
		// our service's dependencies like DB or downstream services
		Dependencies: []health.DependencyCheck{
			func(ctx context.Context) health.Report {
				return health.Report{
					Name: "downstream-service-name",
					Issues: []health.Issue{
						{
							Code:    "xy-db-disconnected",
							Message: "failed to ping the database through the connection",
						},
					},
					Details: map[string]any{
						"http-request-throughput": 42,
					},
					Dependencies: []health.Report{
						{
							Name:   "xy-db",
							Status: health.Down,
						},
					},
				}
			},
		},
		Details: map[string]health.DetailCheck{
			"detail-name": func(ctx context.Context) (any, error) {
				return 42, nil
			},
		},
	}

	ctx := context.Background()
	r := m.HealthCheck(ctx)
	dto, err := dtokit.Map[health.ReportJSONDTO](ctx, r)
	assert.NoError(t, err)

	data, err := json.MarshalIndent(dto, "", "  ")
	assert.NoError(t, err)
	fmt.Println(string(data))

}

func TestReport_WithIssue(t *testing.T) {
	var r health.Report

	expIssue := health.Issue{
		Code:    "the-answer-is",
		Message: "to all questions",
		Causes:  health.Degraded,
	}

	gotReport := r.WithIssue(expIssue)

	assert.NotEqual(t, r, gotReport)
	assert.Empty(t, r.Issues)
	assert.NotEmpty(t, gotReport.Issues)
	assert.Contain(t, gotReport.Issues, expIssue)
}

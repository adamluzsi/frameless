package rfc7807_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"go.llib.dev/frameless/pkg/errorkit"
	rfc78072 "go.llib.dev/frameless/pkg/httpkit/rfc7807"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
	"go.llib.dev/testcase/random"
	"go.llib.dev/testcase/tchttp"
)

func TestHandler(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		baseURL = testcase.LetValue(s, "")
		mapping = testcase.LetValue[rfc78072.HandlerMappingFunc](s, nil)
	)
	subject := testcase.Let(s, func(t *testcase.T) rfc78072.Handler {
		return rfc78072.Handler{
			BaseURL: baseURL.Get(t),
			Mapping: mapping.Get(t),
		}
	})

	s.Describe(".HandleError", func(s *testcase.Spec) {
		var (
			w   = tchttp.LetResponseRecorder(s)
			r   = tchttp.LetServerRequest(s)
			err = let.Error(s)
		)
		act := func(t *testcase.T) {
			subject.Get(t).HandleError(w.Get(t), r.Get(t), err.Get(t))
		}
		respondedWith := func(t *testcase.T) rfc78072.DTO {
			act(t)
			var dto rfc78072.DTO
			t.Log(w.Get(t).Body.String())
			assert.Must(t).NoError(json.Unmarshal(w.Get(t).Body.Bytes(), &dto))
			return dto
		}

		s.Then("it responds back with RFC7807 format encoded in JSON", func(t *testcase.T) {
			act(t)

			var dto rfc78072.DTO
			assert.Must(t).NoError(json.Unmarshal(w.Get(t).Body.Bytes(), &dto))
			assert.Must(t).Equal("internal-server-error", dto.Type.ID)
			assert.Must(t).Empty(dto.Type.BaseURL)
			assert.Must(t).Equal("Internal Server Error", dto.Title)
			assert.Must(t).Equal(http.StatusInternalServerError, dto.Status)
			assert.Must(t).Empty(dto.Detail)
			assert.Must(t).Empty(dto.Instance)
		})

		s.When("BaseURL is as resource path", func(s *testcase.Spec) {
			baseURL.Let(s, func(t *testcase.T) string {
				return "/errors"
			})

			s.Then("the type id value is under that resource path", func(t *testcase.T) {
				dto := respondedWith(t)
				assert.Must(t).Equal(baseURL.Get(t), dto.Type.BaseURL)
			})
		})

		s.When("mapping is provided", func(s *testcase.Spec) {
			var (
				code = let.StringNC(s, 5, random.CharsetAlpha())
				msg  = let.String(s)
				key  = let.StringNC(s, 3, random.CharsetDigit())
			)
			mapping.Let(s, func(t *testcase.T) rfc78072.HandlerMappingFunc {
				return func(ctx context.Context, err error, dto *rfc78072.DTO) {
					assert.Must(t).NotEmpty(dto.Type)
					assert.Must(t).NotEmpty(dto.Title)
					dto.Extensions = ExampleExtensionError{
						Code:    code.Get(t),
						Message: msg.Get(t),
					}
					if v, ok := ctx.Value(key.Get(t)).(string); ok {
						dto.Detail = v
					}
				}
			})

			s.Then("mapping will receive a DTO with some values already configured", func(t *testcase.T) {
				dto := respondedWith(t)
				assert.Must(t).Equal(code.Get(t), dto.Extensions.(map[string]any)["code"].(string))
				assert.Must(t).Equal(msg.Get(t), dto.Extensions.(map[string]any)["message"].(string))
			})

			s.And("error has context attached to it", func(s *testcase.Spec) {
				var (
					val = let.StringNC(s, 3, random.CharsetAlpha())
					ctx = testcase.Let(s, func(t *testcase.T) context.Context {
						return context.WithValue(context.Background(), key.Get(t), val.Get(t))
					})
				)
				err.Let(s, func(t *testcase.T) error {
					return errorkit.WithContext(err.Super(t), ctx.Get(t))
				})

				s.Then("then the mapping will receive this error context", func(t *testcase.T) {
					dto := respondedWith(t)
					assert.Must(t).Equal(val.Get(t), dto.Detail)
				})
			})
		})

		s.When("request is cancelled and error is also a context cancellation", func(s *testcase.Spec) {
			r.Let(s, func(t *testcase.T) *http.Request {
				v := r.Super(t)
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return v.WithContext(ctx)
			})
			err.Let(s, func(t *testcase.T) error {
				return fmt.Errorf("boom: %w", r.Get(t).Context().Err())
			})

			s.Then("no error response is written since nobody listens anymore", func(t *testcase.T) {
				act(t)
				assert.Must(t).Equal(0, w.Get(t).Body.Len())
			})
		})

		s.When("error is a user error", func(s *testcase.Spec) {
			usrErr := testcase.Let(s, func(t *testcase.T) errorkit.UserError {
				return errorkit.UserError{
					Code:    "usr-err",
					Message: "the user error message",
				}
			})
			err.Let(s, func(t *testcase.T) error {
				return usrErr.Get(t)
			})

			s.Then("message is returned as part of detail", func(t *testcase.T) {
				dto := respondedWith(t)
				assert.Contains(t, dto.Detail, usrErr.Get(t).Message)
			})

			s.And("mapping is provided", func(s *testcase.Spec) {
				mapping.Let(s, func(t *testcase.T) rfc78072.HandlerMappingFunc {
					return func(ctx context.Context, err error, dto *rfc78072.DTO) {
						assert.Must(t).Equal(string(usrErr.Get(t).Code), dto.Type.ID)
						assert.Contains(t, dto.Detail, string(usrErr.Get(t).Message))
						assert.Must(t).ErrorIs(usrErr.Get(t), err)
					}
				})

				s.Then("mapping will receive a DTO with some values already configured", func(t *testcase.T) {
					act(t) // assert as part of mapping
				})
			})
		})
	})

	s.Describe(".ToDTO", func(s *testcase.Spec) {
		var (
			ctx = let.Context(s)
			err = let.Error(s)
		)
		act := func(t *testcase.T) rfc78072.DTO {
			return subject.Get(t).ToDTO(ctx.Get(t), err.Get(t))
		}

		s.Then("it responds back with RFC7807 format encoded in JSON", func(t *testcase.T) {
			var dto rfc78072.DTO
			dto = act(t)
			assert.Must(t).NotEmpty(dto)
			assert.Must(t).Equal("internal-server-error", dto.Type.ID)
			assert.Must(t).Empty(dto.Type.BaseURL)
			assert.Must(t).Equal("Internal Server Error", dto.Title)
			assert.Must(t).Equal(http.StatusInternalServerError, dto.Status)
			assert.Must(t).Empty(dto.Detail)
			assert.Must(t).Empty(dto.Instance)
		})

		s.When("BaseURL is as resource path", func(s *testcase.Spec) {
			baseURL.Let(s, func(t *testcase.T) string {
				return "/errors"
			})

			s.Then("the type id value is under that resource path", func(t *testcase.T) {
				dto := act(t)
				assert.Must(t).Equal(baseURL.Get(t), dto.Type.BaseURL)
			})
		})

		s.When("mapping is provided", func(s *testcase.Spec) {
			var (
				code = let.StringNC(s, 5, random.CharsetAlpha())
				msg  = let.String(s)
				key  = let.StringNC(s, 3, random.CharsetDigit())
			)
			mapping.Let(s, func(t *testcase.T) rfc78072.HandlerMappingFunc {
				return func(ctx context.Context, err error, dto *rfc78072.DTO) {
					assert.Must(t).NotEmpty(dto.Type)
					assert.Must(t).NotEmpty(dto.Title)
					dto.Extensions = ExampleExtensionError{
						Code:    code.Get(t),
						Message: msg.Get(t),
					}
					if v, ok := ctx.Value(key.Get(t)).(string); ok {
						dto.Detail = v
					}
				}
			})

			s.Then("mapping will receive a DTO with some values already configured", func(t *testcase.T) {
				dto := act(t)
				assert.Must(t).Equal(code.Get(t), dto.Extensions.(ExampleExtensionError).Code)
				assert.Must(t).Equal(msg.Get(t), dto.Extensions.(ExampleExtensionError).Message)
			})

			s.And("error has context attached to it", func(s *testcase.Spec) {
				var (
					val = let.StringNC(s, 3, random.CharsetAlpha())
					ctx = testcase.Let(s, func(t *testcase.T) context.Context {
						return context.WithValue(context.Background(), key.Get(t), val.Get(t))
					})
				)
				err.Let(s, func(t *testcase.T) error {
					return errorkit.WithContext(err.Super(t), ctx.Get(t))
				})

				s.Then("then the mapping will receive this error context", func(t *testcase.T) {
					dto := act(t)
					assert.Must(t).Equal(val.Get(t), dto.Detail)
				})
			})
		})

		s.When("error is a user error", func(s *testcase.Spec) {
			usrErr := testcase.Let(s, func(t *testcase.T) errorkit.UserError {
				return errorkit.UserError{
					Code:    "usr-err",
					Message: "the user error message",
				}
			})
			err.Let(s, func(t *testcase.T) error {
				return usrErr.Get(t)
			})

			s.Then("message is returned as part of detail", func(t *testcase.T) {
				dto := act(t)
				assert.Contains(t, dto.Detail, usrErr.Get(t).Message)
			})

			s.And("mapping is provided", func(s *testcase.Spec) {
				mapping.Let(s, func(t *testcase.T) rfc78072.HandlerMappingFunc {
					return func(ctx context.Context, err error, dto *rfc78072.DTO) {
						assert.Must(t).Equal(string(usrErr.Get(t).Code), dto.Type.ID)
						assert.Contains(t, dto.Detail, string(usrErr.Get(t).Message))
						assert.Must(t).ErrorIs(usrErr.Get(t), err)
					}
				})

				s.Then("mapping will receive a DTO with some values already configured", func(t *testcase.T) {
					act(t) // assert as part of mapping
				})
			})
		})
	})
}

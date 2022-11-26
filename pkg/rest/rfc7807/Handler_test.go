package rfc7807_test

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/adamluzsi/frameless/pkg/errorutil"
	rfc78072 "github.com/adamluzsi/frameless/pkg/rest/rfc7807"
	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/httpspec"
	"github.com/adamluzsi/testcase/let"
	"github.com/adamluzsi/testcase/random"
	"net/http"
	"testing"
)

func TestHandler(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		baseURL = testcase.LetValue(s, "")
		mapping = testcase.LetValue[rfc78072.HandlerMappingFunc[ExampleExtension]](s, nil)
	)
	subject := testcase.Let(s, func(t *testcase.T) rfc78072.Handler[ExampleExtension] {
		return rfc78072.Handler[ExampleExtension]{
			BaseURL: baseURL.Get(t),
			Mapping: mapping.Get(t),
		}
	})

	s.Describe(".HandleError", func(s *testcase.Spec) {
		var (
			w   = httpspec.LetResponseRecorder(s)
			r   = httpspec.LetRequest(s, httpspec.RequestVar{})
			err = let.Error(s)
		)
		act := func(t *testcase.T) {
			subject.Get(t).HandleError(w.Get(t), r.Get(t), err.Get(t))
		}
		respondedWith := func(t *testcase.T) rfc78072.DTO[ExampleExtension] {
			act(t)
			var dto rfc78072.DTO[ExampleExtension]
			t.Log(w.Get(t).Body.String())
			t.Must.NoError(json.Unmarshal(w.Get(t).Body.Bytes(), &dto))
			return dto
		}

		s.Then("it responds back with RFC7807 format encoded in JSON", func(t *testcase.T) {
			act(t)

			var dto rfc78072.DTO[ExampleExtension]
			t.Must.NoError(json.Unmarshal(w.Get(t).Body.Bytes(), &dto))
			t.Must.Equal("internal-server-error", dto.Type.ID)
			t.Must.Empty(dto.Type.BaseURL)
			t.Must.Equal("Internal Server Error", dto.Title)
			t.Must.Equal(http.StatusInternalServerError, dto.Status)
			t.Must.Empty(dto.Detail)
			t.Must.Empty(dto.Instance)
		})

		s.When("BaseURL is as resource path", func(s *testcase.Spec) {
			baseURL.Let(s, func(t *testcase.T) string {
				return "/errors"
			})

			s.Then("the type id value is under that resource path", func(t *testcase.T) {
				dto := respondedWith(t)
				t.Must.Equal(baseURL.Get(t), dto.Type.BaseURL)
			})
		})

		s.When("mapping is provided", func(s *testcase.Spec) {
			var (
				code = let.StringNC(s, 5, random.CharsetAlpha())
				msg  = let.String(s)
				key  = let.StringNC(s, 3, random.CharsetDigit())
			)
			mapping.Let(s, func(t *testcase.T) rfc78072.HandlerMappingFunc[ExampleExtension] {
				return func(ctx context.Context, err error, dto *rfc78072.DTO[ExampleExtension]) {
					t.Must.NotEmpty(dto.Type)
					t.Must.NotEmpty(dto.Title)
					dto.Extensions.Error.Code = code.Get(t)
					dto.Extensions.Error.Message = msg.Get(t)
					if v, ok := ctx.Value(key.Get(t)).(string); ok {
						dto.Detail = v
					}
				}
			})

			s.Then("mapping will receive a DTO with some values already configured", func(t *testcase.T) {
				dto := respondedWith(t)
				t.Must.Equal(code.Get(t), dto.Extensions.Error.Code)
				t.Must.Equal(msg.Get(t), dto.Extensions.Error.Message)
			})

			s.And("error has context attached to it", func(s *testcase.Spec) {
				var (
					val = let.StringNC(s, 3, random.CharsetAlpha())
					ctx = testcase.Let(s, func(t *testcase.T) context.Context {
						return context.WithValue(context.Background(), key.Get(t), val.Get(t))
					})
				)
				err.Let(s, func(t *testcase.T) error {
					return errorutil.With(err.Super(t)).Context(ctx.Get(t))
				})

				s.Then("then the mapping will receive this error context", func(t *testcase.T) {
					dto := respondedWith(t)
					t.Must.Equal(val.Get(t), dto.Detail)
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
				t.Must.Equal(0, w.Get(t).Body.Len())
			})
		})

		s.When("error is a user error", func(s *testcase.Spec) {
			usrErr := testcase.Let(s, func(t *testcase.T) errorutil.UserError {
				return errorutil.UserError{
					ID:      "usr-err",
					Message: "the user error message",
				}
			})
			err.Let(s, func(t *testcase.T) error {
				return usrErr.Get(t)
			})

			s.Then("message is returned as part of detail", func(t *testcase.T) {
				dto := respondedWith(t)
				t.Must.Contain(dto.Detail, usrErr.Get(t).Message)
			})

			s.And("mapping is provided", func(s *testcase.Spec) {
				mapping.Let(s, func(t *testcase.T) rfc78072.HandlerMappingFunc[ExampleExtension] {
					return func(ctx context.Context, err error, dto *rfc78072.DTO[ExampleExtension]) {
						t.Must.Equal(string(usrErr.Get(t).ID), dto.Type.ID)
						t.Must.Contain(dto.Detail, string(usrErr.Get(t).Message))
						t.Must.ErrorIs(usrErr.Get(t), err)
					}
				})

				s.Then("mapping will receive a DTO with some values already configured", func(t *testcase.T) {
					act(t) // assert as part of mapping
				})
			})

			s.And("error has detail", func(s *testcase.Spec) {
				detail := let.String(s)

				err.Let(s, func(t *testcase.T) error {
					return errorutil.With(err.Super(t)).Detail(detail.Get(t))
				})

				s.Then("user error message is part of the reply detail", func(t *testcase.T) {
					dto := respondedWith(t)
					t.Must.Contain(dto.Detail, detail.Get(t))
				})

				s.Then("error detail is part of the reply detail", func(t *testcase.T) {
					dto := respondedWith(t)
					t.Must.Contain(dto.Detail, detail.Get(t))
				})
			})
		})

		s.When("error has detail", func(s *testcase.Spec) {
			detail := let.String(s)

			err.Let(s, func(t *testcase.T) error {
				return errorutil.With(err.Super(t)).Detail(detail.Get(t))
			})

			s.Then("detail is returned", func(t *testcase.T) {
				dto := respondedWith(t)
				t.Must.Equal(detail.Get(t), dto.Detail)
			})

			s.And("mapping is provided", func(s *testcase.Spec) {
				mapping.Let(s, func(t *testcase.T) rfc78072.HandlerMappingFunc[ExampleExtension] {
					return func(ctx context.Context, err error, dto *rfc78072.DTO[ExampleExtension]) {
						t.Must.Contain(detail.Get(t), dto.Detail)
					}
				})

				s.Then("mapping will receive a DTO with some values already configured", func(t *testcase.T) {
					act(t) // assert as part of mapping
				})
			})
		})
	})
}

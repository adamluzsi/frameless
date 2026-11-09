package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"restapi/adapter/httpapi"
	"restapi/domain/mydomain"
	"strings"
	"testing"

	"go.llib.dev/frameless/adapter/memory"
	"go.llib.dev/frameless/pkg/httpkit"
	"go.llib.dev/frameless/pkg/httpkit/mediatype"
	"go.llib.dev/frameless/pkg/httpkit/rfc7807"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/random"
)

func TestMakeHandler(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		userRepository = testcase.Let(s, func(t *testcase.T) *memory.Repository[mydomain.User, mydomain.UserID] {
			return &memory.Repository[mydomain.User, mydomain.UserID]{}
		})
		noteRepository = testcase.Let(s, func(t *testcase.T) *memory.Repository[mydomain.Note, mydomain.NoteID] {
			return &memory.Repository[mydomain.Note, mydomain.NoteID]{}
		})
	)

	subject := testcase.Let(s, func(t *testcase.T) http.Handler {
		return httpapi.MakeHandler(httpapi.Config{
			UserRepository: userRepository.Get(t),
			NoteRepository: noteRepository.Get(t),
		})
	})

	user := testcase.Let(s, func(t *testcase.T) mydomain.User {
		u := mydomain.User{Username: t.Random.StringNC(8, random.CharsetAlpha())}
		assert.NoError(t, userRepository.Get(t).Create(context.Background(), &u))
		return u
	}).EagerLoading(s)

	s.Describe("#ServeHTTP", func(s *testcase.Spec) {
		var (
			method = testcase.LetValue(s, http.MethodGet)
			path   = testcase.LetValue(s, "/users")
			body   = testcase.LetValue(s, "")
		)
		act := func(t *testcase.T) *httptest.ResponseRecorder {
			req := httptest.NewRequest(method.Get(t), path.Get(t), strings.NewReader(body.Get(t)))
			if body.Get(t) != "" {
				req.Header.Set("Content-Type", mediatype.JSON)
			}
			rr := httptest.NewRecorder()
			subject.Get(t).ServeHTTP(rr, req)
			t.OnFail(func() {
				t.Log(method.Get(t), path.Get(t), rr.Code)
				t.Log(rr.Body.String())
			})
			return rr
		}

		s.Then("it lists the users in their JSON format", func(t *testcase.T) {
			rr := act(t)
			assert.Equal(t, http.StatusOK, rr.Code)

			assert.Equal(t, []map[string]any{{
				"id":       string(user.Get(t).ID),
				"username": user.Get(t).Username,
			}}, decodeJSON[[]map[string]any](t, rr))
		})

		s.When("a user is requested by its ID", func(s *testcase.Spec) {
			path.Let(s, func(t *testcase.T) string {
				return "/users/" + string(user.Get(t).ID)
			})

			s.Then("it shows the user in its JSON format", func(t *testcase.T) {
				rr := act(t)
				assert.Equal(t, http.StatusOK, rr.Code)

				assert.Equal(t, map[string]any{
					"id":       string(user.Get(t).ID),
					"username": user.Get(t).Username,
				}, decodeJSON[map[string]any](t, rr))
			})
		})

		s.When("the requested user doesn't exist", func(s *testcase.Spec) {
			path.Let(s, func(t *testcase.T) string {
				return "/users/" + t.Random.StringNC(8, random.CharsetAlpha())
			})

			s.Then("it replies with entity not found", func(t *testcase.T) {
				rr := act(t)
				assert.Equal(t, http.StatusNotFound, rr.Code)

				var dto rfc7807.DTO
				assert.NoError(t, json.Unmarshal(rr.Body.Bytes(), &dto))
				assert.Equal(t, httpkit.ErrEntityNotFound.Code.String(), dto.Type.ID)
			})
		})

		s.When("a user is created from its JSON format", func(s *testcase.Spec) {
			username := testcase.Let(s, func(t *testcase.T) string {
				return t.Random.StringNC(8, random.CharsetAlpha())
			})
			method.LetValue(s, http.MethodPost)
			body.Let(s, func(t *testcase.T) string {
				return `{"username":` + mustJSON(t, username.Get(t)) + `}`
			})

			s.Then("it stores the user and replies with it in its JSON format", func(t *testcase.T) {
				rr := act(t)
				assert.Equal(t, http.StatusCreated, rr.Code)

				got := decodeJSON[map[string]any](t, rr)
				id, _ := got["id"].(string)
				assert.NotEmpty(t, id)
				assert.Equal(t, map[string]any{"id": id, "username": username.Get(t)}, got)

				stored, found, err := userRepository.Get(t).FindByID(context.Background(), mydomain.UserID(id))
				assert.NoError(t, err)
				assert.True(t, found)
				assert.Equal(t, username.Get(t), stored.Username)
			})
		})

		s.When("a note is created for a user from its JSON format", func(s *testcase.Spec) {
			var (
				title    = testcase.Let(s, func(t *testcase.T) string { return t.Random.String() })
				noteBody = testcase.Let(s, func(t *testcase.T) string { return t.Random.String() })
				userID   = testcase.LetValue(s, "")
			)
			method.LetValue(s, http.MethodPost)
			path.Let(s, func(t *testcase.T) string {
				return "/users/" + string(user.Get(t).ID) + "/notes"
			})
			body.Let(s, func(t *testcase.T) string {
				members := []string{
					`"title":` + mustJSON(t, title.Get(t)),
					`"body":` + mustJSON(t, noteBody.Get(t)),
				}
				if userID.Get(t) != "" {
					members = append(members, `"user_id":`+mustJSON(t, userID.Get(t)))
				}
				return "{" + strings.Join(members, ",") + "}"
			})

			s.Then("it stores the note of the user and replies with it in its JSON format", func(t *testcase.T) {
				rr := act(t)
				assert.Equal(t, http.StatusCreated, rr.Code)

				got := decodeJSON[map[string]any](t, rr)
				id, _ := got["id"].(string)
				assert.NotEmpty(t, id)
				assert.Equal(t, map[string]any{
					"id":      id,
					"title":   title.Get(t),
					"body":    noteBody.Get(t),
					"user_id": string(user.Get(t).ID),
				}, got)

				stored, found, err := noteRepository.Get(t).FindByID(context.Background(), mydomain.NoteID(id))
				assert.NoError(t, err)
				assert.True(t, found)
				assert.Equal(t, mydomain.Note{
					ID:     mydomain.NoteID(id),
					Title:  title.Get(t),
					Body:   noteBody.Get(t),
					UserID: user.Get(t).ID,
				}, stored)
			})

			s.And("the request body names another user", func(s *testcase.Spec) {
				userID.Let(s, func(t *testcase.T) string {
					return t.Random.StringNC(8, random.CharsetAlpha())
				})

				s.Then("the note still belongs to the user of the path", func(t *testcase.T) {
					rr := act(t)
					assert.Equal(t, http.StatusCreated, rr.Code)

					got := decodeJSON[map[string]any](t, rr)
					assert.Equal[any](t, string(user.Get(t).ID), got["user_id"])

					id, _ := got["id"].(string)
					stored, found, err := noteRepository.Get(t).FindByID(context.Background(), mydomain.NoteID(id))
					assert.NoError(t, err)
					assert.True(t, found)
					assert.Equal(t, user.Get(t).ID, stored.UserID)
				})
			})
		})

		s.When("the notes of a user are listed", func(s *testcase.Spec) {
			note := testcase.Let(s, func(t *testcase.T) mydomain.Note {
				n := mydomain.Note{
					Title:  t.Random.String(),
					Body:   t.Random.String(),
					UserID: user.Get(t).ID,
				}
				assert.NoError(t, noteRepository.Get(t).Create(context.Background(), &n))
				return n
			}).EagerLoading(s)
			s.Before(func(t *testcase.T) {
				othersNote := mydomain.Note{
					Title:  t.Random.String(),
					Body:   t.Random.String(),
					UserID: mydomain.UserID(t.Random.StringNC(8, random.CharsetAlpha())),
				}
				assert.NoError(t, noteRepository.Get(t).Create(context.Background(), &othersNote))
			})
			path.Let(s, func(t *testcase.T) string {
				return "/users/" + string(user.Get(t).ID) + "/notes"
			})

			s.Then("it lists only the notes of the user in their JSON format", func(t *testcase.T) {
				rr := act(t)
				assert.Equal(t, http.StatusOK, rr.Code)

				assert.Equal(t, []map[string]any{{
					"id":      string(note.Get(t).ID),
					"title":   note.Get(t).Title,
					"body":    note.Get(t).Body,
					"user_id": string(user.Get(t).ID),
				}}, decodeJSON[[]map[string]any](t, rr))
			})
		})
	})
}

func decodeJSON[T any](t *testcase.T, rr *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	assert.Must(t).NoError(json.Unmarshal(rr.Body.Bytes(), &v), assert.Message(rr.Body.String()))
	return v
}

func mustJSON(t *testcase.T, v any) string {
	t.Helper()
	data, err := json.Marshal(v)
	assert.Must(t).NoError(err)
	return string(data)
}

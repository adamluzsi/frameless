package workflow_test

import (
	"errors"
	"fmt"
	"testing"

	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/frameless/pkg/workflow/wftest"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
)

func TestErrParticipantSignatureMismatch(t *testing.T) {
	s := testcase.NewSpec(t)

	id := wftest.LetParticipantID(s)
	cause := let.Error(s)
	subject := let.Var(s, func(t *testcase.T) workflow.ErrParticipantSignatureMismatch {
		return workflow.ErrParticipantSignatureMismatch{ID: id.Get(t), Cause: cause.Get(t)}
	})
	act := func(t *testcase.T) error { return fmt.Errorf("wrapped: %w", subject.Get(t)) }

	s.Then("exposes the participant ID and cause through errors.As and Unwrap", func(t *testcase.T) {
		var mismatch workflow.ErrParticipantSignatureMismatch
		assert.True(t, errors.As(act(t), &mismatch))
		assert.Equal(t, mismatch.ID, id.Get(t))
		assert.ErrorIs(t, mismatch.Unwrap(), cause.Get(t))
		assert.ErrorIs(t, act(t), cause.Get(t))
		assert.Contains(t, act(t).Error(), string(id.Get(t)))
		assert.Contains(t, act(t).Error(), cause.Get(t).Error())
	})
	s.Then("matches the same participant ID independently of the cause", func(t *testcase.T) {
		assert.ErrorIs(t, act(t), workflow.ErrParticipantSignatureMismatch{ID: id.Get(t)})
		assert.ErrorIs(t, act(t), &workflow.ErrParticipantSignatureMismatch{ID: id.Get(t)})
		assert.False(t, errors.Is(act(t), workflow.ErrParticipantSignatureMismatch{ID: id.Get(t) + "-other"}))
	})
	s.When("there is no cause", func(s *testcase.Spec) {
		cause.LetValue(s, nil)
		s.Then("is still a usable typed error", func(t *testcase.T) {
			assert.NotEmpty(t, act(t).Error())
			assert.Nil(t, subject.Get(t).Unwrap())
			assert.False(t, workflow.ErrIsFatal(act(t)))
		})
	})
	s.When("the legacy invalid-function cause is fatal on its own", func(s *testcase.Spec) {
		cause.LetValue(s, workflow.ErrInvalidParticipantFunc)
		s.Then("the mismatch remains nonfatal while preserving legacy error access", func(t *testcase.T) {
			assert.ErrorIs(t, act(t), workflow.ErrInvalidParticipantFunc)
			assert.False(t, workflow.ErrIsFatal(act(t)))
			mismatch := subject.Get(t)
			wrappedPointer := fmt.Errorf("wrapped: %w", &mismatch)
			assert.False(t, workflow.ErrIsFatal(wrappedPointer))
			var pointer *workflow.ErrParticipantSignatureMismatch
			assert.True(t, errors.As(wrappedPointer, &pointer))
			assert.Equal(t, pointer.ID, id.Get(t))
		})
	})
}

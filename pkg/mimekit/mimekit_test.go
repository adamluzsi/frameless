package mimekit_test

import (
	"testing"

	"go.llib.dev/frameless/pkg/mimekit"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/random"
)

func Test(t *testing.T) {
	s := testcase.NewSpec(t)

	mimetype := testcase.Let(s, func(t *testcase.T) string {
		return random.Pick(t.Random,
			mimekit.PlainText,
			mimekit.JSON,
			mimekit.XML,
			mimekit.HTML,
			mimekit.OctetStream,
			mimekit.FormUrlencoded,
			"?UnknownType?",
		)
	})

	s.Describe("#MediaType", func(s *testcase.Spec) {
		act := func(t *testcase.T) string {
			return mimekit.MediaType(mimetype.Get(t))
		}

		s.Then("non empty result returned on a non empty media type", func(t *testcase.T) {
			assert.NotEmpty(t, act(t))
		})

		s.When("subject contains parameters", func(s *testcase.Spec) {
			mimetype.LetValue(s, "text/html; charset=UTF-8")

			s.Then("the base type is returned", func(t *testcase.T) {
				assert.Equal(t, act(t), "text/html")
			})
		})

		s.When("subject is a base media type only", func(s *testcase.Spec) {
			mimetype.LetValue(s, "text/html;")

			s.Then("media type is returned", func(t *testcase.T) {
				assert.Equal(t, act(t), "text/html")
			})
		})
	})
}

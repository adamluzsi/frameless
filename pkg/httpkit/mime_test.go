package httpkit

import (
	"testing"

	"go.llib.dev/frameless/pkg/httpkit/mediatype"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/random"
)

func Test_lookupMediaType(t *testing.T) {
	s := testcase.NewSpec(t)

	mimetype := testcase.Let(s, func(t *testcase.T) string {
		return random.Pick(t.Random,
			mediatype.PlainText,
			mediatype.JSON,
			mediatype.XML,
			mediatype.HTML,
			mediatype.OctetStream,
			mediatype.FormUrlencoded,
			"?UnknownType?",
		)
	})

	act := func(t *testcase.T) (mediatype.MediaType, bool) {
		t.OnFail(func() { t.Log(mimetype.Get(t)) })
		return lookupMediaType(mimetype.Get(t))
	}

	s.Then("non-empty result returned on a non-empty media type", func(t *testcase.T) {
		mt, ok := act(t)
		assert.True(t, ok)
		assert.NotEmpty(t, mt)
	})

	s.When("subject contains parameters", func(s *testcase.Spec) {
		mimetype.LetValue(s, "text/html; charset=UTF-8")

		s.Then("the base type is returned", func(t *testcase.T) {
			mt, ok := act(t)
			assert.True(t, ok)
			assert.NotEmpty(t, mt)
			assert.Equal(t, mt, "text/html")
		})
	})

	s.When("subject is a base media type only", func(s *testcase.Spec) {
		mimetype.LetValue(s, "text/html;")

		s.Then("media type is returned", func(t *testcase.T) {
			mt, ok := act(t)
			assert.True(t, ok)
			assert.Equal(t, mt, "text/html")
		})
	})
}

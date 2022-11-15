package errorutil_test

import (
	"github.com/adamluzsi/frameless/pkg/errorutil"
	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/let"
	"testing"
)

func TestTag(t *testing.T) {
	s := testcase.NewSpec(t)
	baseErr := let.Error(s)

	type (
		MyTag  struct{}
		OthTag struct{}
	)

	s.Test("when no tag is given", func(t *testcase.T) {
		gotErr := baseErr.Get(t)
		t.Must.False(errorutil.HasTag(gotErr, MyTag{}))
		t.Must.False(errorutil.HasTag(gotErr, OthTag{}))
	})

	s.Test("when a tag is given", func(t *testcase.T) {
		gotErr := errorutil.Tag(baseErr.Get(t), MyTag{})
		t.Must.ErrorIs(baseErr.Get(t), gotErr)
		t.Must.True(errorutil.HasTag(gotErr, MyTag{}))
		t.Must.False(errorutil.HasTag(gotErr, OthTag{}))
	})

	s.Test("when multiple tags are given", func(t *testcase.T) {
		gotErr := errorutil.Tag(baseErr.Get(t), MyTag{})
		gotErr = errorutil.Tag(gotErr, OthTag{})
		t.Must.ErrorIs(baseErr.Get(t), gotErr)
		t.Must.True(errorutil.HasTag(gotErr, MyTag{}))
		t.Must.True(errorutil.HasTag(gotErr, OthTag{}))
	})
}

package errorkit_test

import (
	"testing"

	"github.com/adamluzsi/frameless/pkg/errorkit"
	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/let"
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
		t.Must.False(errorkit.HasTag(gotErr, MyTag{}))
		t.Must.False(errorkit.HasTag(gotErr, OthTag{}))
	})

	s.Test("when a tag is given", func(t *testcase.T) {
		gotErr := errorkit.Tag(baseErr.Get(t), MyTag{})
		t.Must.ErrorIs(baseErr.Get(t), gotErr)
		t.Must.True(errorkit.HasTag(gotErr, MyTag{}))
		t.Must.False(errorkit.HasTag(gotErr, OthTag{}))
	})

	s.Test("when multiple tags are given", func(t *testcase.T) {
		gotErr := errorkit.Tag(baseErr.Get(t), MyTag{})
		gotErr = errorkit.Tag(gotErr, OthTag{})
		t.Must.ErrorIs(baseErr.Get(t), gotErr)
		t.Must.True(errorkit.HasTag(gotErr, MyTag{}))
		t.Must.True(errorkit.HasTag(gotErr, OthTag{}))
	})
}

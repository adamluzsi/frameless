package spechelper

import (
	"context"
	"testing"

	"github.com/adamluzsi/testcase"
)

func MakeCtx(testing.TB) context.Context { return context.Background() }

func MakeString(tb testing.TB) string {
	return tb.(*testcase.T).Random.String()
}


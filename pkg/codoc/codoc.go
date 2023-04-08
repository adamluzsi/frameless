// Package codoc helps makes maintainable the software documentation process.
package codoc

import (
	"context"
	"github.com/adamluzsi/frameless/pkg/logger"
	"os"
	"strconv"
)

// Explain allows you to explain the high level business logic of a given function block.
// You should refrain explaining the implementation details, but focus on the high level behaviour.
func Explain(ctx context.Context, desc string, opts ...explainOpt) {
	if !enabled {
		return
	}
}

type explainOptions struct {
	Participants []string
}

type explainOpt interface{ configure(*explainOptions) }

//////////////////////////////////////////////////////// Context ///////////////////////////////////////////////////////

func Context(ctx context.Context) (_ context.Context, cancel func()) {
	if _, ok := ctx.Value(ctxKey{}).(*contextOptions); ok {
		return ctx, func() {}
	}
	return context.WithValue(ctx, ctxKey{}, &contextOptions{}), func() {}
}

type (
	ctxKey   struct{}
	ctxValue struct{}
)

type contextOptions struct {
	TraceID string
}

type ctxOpt interface{ configure(*contextOptions) }

////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

////////////////////////////////////////////////////////// ENV /////////////////////////////////////////////////////////

var enabled bool

func init() {
	const codocEnvVarKey = "CODOC_ENABLED"
	ceraw, ok := os.LookupEnv(codocEnvVarKey)
	if !ok {
		return
	}
	v, err := strconv.ParseBool(ceraw)
	if err != nil {
		const invalidBoolValWarnMsg = "invalid boolean value set to the " + codocEnvVarKey + " environment variable"
		logger.Warn(context.Background(), invalidBoolValWarnMsg, logger.Field("value", ceraw))
		return
	}
	enabled = v
}

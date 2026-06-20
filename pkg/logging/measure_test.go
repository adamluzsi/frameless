package logging_test

import (
	"context"
	"testing"
	"time"

	"go.llib.dev/frameless/pkg/logger"
	"go.llib.dev/frameless/pkg/logging"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/let"
)

func ExampleMeasure() {
	ctx := context.Background()
	m := logging.Measure()

	logger.Info(ctx, "start x")
	defer logger.Info(ctx, "finish x", logging.LazyDetail(m))

	logger.Info(ctx, "x", m())
	time.Sleep(time.Second)
	logger.Info(ctx, "x + 1s", m())
}

func TestMeasure(t *testing.T) {
	s := testcase.NewSpec(t)

	act := let.Act(func(t *testcase.T) func() logging.Detail {
		return logging.Measure()
	})
}

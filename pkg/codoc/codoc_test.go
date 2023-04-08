package codoc_test

import (
	"context"
	"github.com/adamluzsi/frameless/pkg/codoc"
)

func ExampleExplain() {
	ctx := context.Background()

	codoc.Explain(ctx, "my high level explanation about the business logic a given function delivers")
}

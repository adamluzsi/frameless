//go:build testoutput

package main_test

import (
	"github.com/adamluzsi/frameless/pkg/logger"
	"testing"
)

func TestOutputSuppression(t *testing.T) {
	logger.Info(nil, "from spike")
}

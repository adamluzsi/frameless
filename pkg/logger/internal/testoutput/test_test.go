package main_test

import (
	"github.com/adamluzsi/testcase/assert"
	"os"
	"os/exec"
	"path"
	"testing"
)

func TestGoTest_loggingIsSuppressed(t *testing.T) {
	CheckPWD(t)

	cmd := exec.Command("go", "test", "-tags", "testoutput", "-run", "TestOutputSuppression")
	output, err := cmd.CombinedOutput()
	assert.NoError(t, err, string(output))
	assert.NotEmpty(t, output)
	assert.NotContain(t, string(output), "no tests to run")
	assert.NotContain(t, string(output), `"level":"`)
	assert.NotContain(t, string(output), `"message":"`)
}

func TestGoRun_loggingIsNotSuppressed(t *testing.T) {
	CheckPWD(t)

	cmd := exec.Command("go", "run", ".")
	output, err := cmd.CombinedOutput()
	assert.NoError(t, err, string(output))
	assert.NotEmpty(t, output)
	assert.Contain(t, string(output), `"level":"`)
	assert.Contain(t, string(output), `"message":"`)
}

func CheckPWD(tb testing.TB) {
	pwd, err := os.Getwd()
	assert.NoError(tb, err)
	if path.Base(pwd) != "testoutput" {
		tb.Skip("incorrect pwd")
	}
}

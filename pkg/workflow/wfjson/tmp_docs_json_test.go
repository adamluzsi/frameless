package wfjson_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/frameless/pkg/workflow/wfjson"
)

func TestTMPDocsJSONExamples(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	var files []string
	for _, pattern := range []string{
		"../*.md",
		"../docs/*.md",
		"../../../.agents/skills/frameless-workflow/*.md",
		"../../../.agents/skills/frameless-workflow/references/*.md",
	} {
		ms, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, ms...)
	}
	c := wfjson.NewCodec()
	var total, skipped, failed int
	for _, file := range files {
		t.Log(file)

		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(string(raw), "\n")
		for i := 0; i < len(lines); i++ {
			if strings.TrimSpace(lines[i]) != "```json" {
				continue
			}
			start := i + 1
			var block []string
			for i++; i < len(lines) && strings.TrimSpace(lines[i]) != "```"; i++ {
				block = append(block, lines[i])
			}
			data := strings.Join(block, "\n")
			total++
			if strings.Contains(data, "…") || strings.Contains(data, "...") || strings.Contains(data, "acme::") {
				skipped++
				continue
			}
			t.Log("\n", data, "\n")
			var errs []string
			ok := false
			{
				var v workflow.Definition
				if err := c.Unmarshal([]byte(data), &v); err == nil {
					ok = true
				} else {
					errs = append(errs, "Definition: "+err.Error())
				}
			}
			if !ok {
				var v workflow.Condition
				if err := c.Unmarshal([]byte(data), &v); err == nil {
					ok = true
				} else {
					errs = append(errs, "Condition: "+err.Error())
				}
			}
			if !ok {
				var v workflow.Event
				if err := c.Unmarshal([]byte(data), &v); err == nil {
					ok = true
				} else {
					errs = append(errs, "Event: "+err.Error())
				}
			}
			if !ok {
				var v []workflow.Event
				if err := c.Unmarshal([]byte(data), &v); err == nil {
					ok = true
				} else {
					errs = append(errs, "[]Event: "+err.Error())
				}
			}
			if !ok {
				failed++
				t.Errorf("%s:%d does not decode:\n%s\n  %s", file, start, data, strings.Join(errs, "\n  "))
			}
		}
	}
	t.Logf("json blocks: total=%d skipped=%d failed=%d", total, skipped, failed)
}

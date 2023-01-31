package xtemplate_test

import (
	"github.com/adamluzsi/frameless/pkg/xtemplate"
	"html/template"
	"testing"
)

func TestRenderer_Render(t *testing.T) {

	r := xtemplate.Renderer[template.Template]{}

	_ = r
}

package xtemplate

import (
	"fmt"
	htmlTemplate "html/template"
	"io"
	"io/fs"
	"path"
	textTemplate "text/template"
)

type Renderer[Template templateType] struct {
	FS         fs.FS
	LayoutPath string

	cache map[string]*Template
}

type templateType interface {
	textTemplate.Template | htmlTemplate.Template
}

type templateInstance interface {
	New(name string) templateInstance
	Parse(text string) (templateInstance, error)
	ExecuteTemplate(wr io.Writer, name string, data any) error
}

const (
	LayoutName  = `layout`
	ContentName = `content`
)

func (r Renderer[Template]) Render(w io.Writer, name string, data any) error {
	tmpl := r.newTemplate(nil, path.Join("page", name))

	//if err := r.parseLayout(tmpl); err != nil {
	//	return err
	//}
	//
	//bs, err := r.readTempl(name)
	//if err != nil {
	//	return err
	//}
	//
	//body := fmt.Sprintf(`{{ define %q }}%s{{ end }}`, ContentName, string(bs))
	//_, err = tmpl.New(name).Parse(body)
	//if err != nil {
	//	return err
	//}

	return tmpl.ExecuteTemplate(w, "layout", data)
}

//func (r Renderer[Template]) parseLayout(tmpl *Template) error {
//	if r.LayoutPath == "" {
//		htmlTemplate.New("").Parse()
//		_, err := tmpl.New(LayoutName).Parse(fmt.Sprintf(`{{ template %q . }}`, ContentName))
//		return err
//	}
//	_, err := tmpl.New(LayoutName).Parse(string(bs))
//	return err
//}
//
//func (r Renderer[Template]) readTempl(name string) (_ []byte, rErr error) {
//	f, err := r.FS.Open(path.Join(name))
//	if err != nil {
//		return nil, err
//	}
//	defer func() { rErr = errorutil.Merge(rErr, f.Close()) }()
//
//	all, err := io.ReadAll(f)
//	if err != nil {
//		return nil, err
//	}
//
//	return all, nil
//}

func (r Renderer[Template]) newTemplate(tmpl *Template, name string) templateInstance[Template] {
	switch v := any(new(Template)).(type) {
	case *textTemplate.Template:
		if tmpl == nil {
			return textTemplate.New(name)
		}
		return any(tmpl).(*textTemplate.Template).New(name)

	case *htmlTemplate.Template:
		if tmpl == nil {
			return htmlTemplate.New(name)
		}
		return any(tmpl).(*htmlTemplate.Template).New(name)

	default:
		panic(fmt.Sprintf("%T template type is not supported", v))
	}
}

func (r Renderer[Template]) newTemplate(tmpl *Template, name string) *Template {
	return func() any {
		switch v := any(new(Template)).(type) {
		case *textTemplate.Template:
			if tmpl == nil {
				return textTemplate.New(name)
			}
			return any(tmpl).(*textTemplate.Template).New(name)

		case *htmlTemplate.Template:
			if tmpl == nil {
				return htmlTemplate.New(name)
			}
			return any(tmpl).(*htmlTemplate.Template).New(name)

		default:
			panic(fmt.Sprintf("%T template type is not supported", v))
		}
	}.(*Template)
}


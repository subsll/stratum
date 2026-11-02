package report

import (
	htmltemplate "html/template"
	"testing"
	texttemplate "text/template"
)

func TestTemplatesParse(t *testing.T) {
	if _, err := texttemplate.New("text").Funcs(textFuncs()).Parse(textTemplate); err != nil {
		t.Fatalf("text template: %v", err)
	}
	if _, err := texttemplate.New("md").Funcs(textFuncs()).Parse(mdTemplate); err != nil {
		t.Fatalf("md template: %v", err)
	}
	if _, err := htmltemplate.New("html").Funcs(htmlFuncs()).Parse(htmlTemplate); err != nil {
		t.Fatalf("html template: %v", err)
	}
}

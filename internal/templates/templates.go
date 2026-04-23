package templates

import (
	"embed"
	"html/template"
	"io"
)

//go:embed *.html
var files embed.FS

// Render writes the named template to w with the given data.
// Each call parses base.html + the named file so that block definitions
// don't bleed across different pages.
func Render(w io.Writer, name string, data any) error {
	t, err := template.New("").ParseFS(files, "base.html", name)
	if err != nil {
		return err
	}
	return t.ExecuteTemplate(w, name, data)
}

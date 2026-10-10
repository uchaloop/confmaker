package confexport

import (
	c "github.com/uchaloop/confmaker/v2"
	"io"
)

type testLoader struct{ *c.Loader }

func makeTestLoader(opts ...c.LoaderOption) *testLoader { return &testLoader{c.MakeLoader(opts...)} }
func (l *testLoader) WriteEnvExample(w io.Writer) error {
	r, e := l.Manifest(c.IncludeDefaults())
	if e != nil {
		return e
	}
	return WriteEnvExample(w, r)
}
func (l *testLoader) WriteManifestJSON(w io.Writer) error {
	r, e := l.Manifest(c.IncludeDefaults())
	if e != nil {
		return e
	}
	return WriteJSON(w, r)
}
func (l *testLoader) WriteManifestMarkdown(w io.Writer) error {
	r, e := l.Manifest(c.IncludeDefaults())
	if e != nil {
		return e
	}
	return WriteMarkdown(w, r)
}
func singleManifestForTest[T any](name string) ([]c.Variable, error) {
	r, e := c.Manifest[T](name, c.IncludeDefaults())
	if e != nil {
		return nil, e
	}
	return r.Configs[0].Variables, nil
}

type failingDefaultText string

func (*failingDefaultText) UnmarshalText([]byte) error { return nil }

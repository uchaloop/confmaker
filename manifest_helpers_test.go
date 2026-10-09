package confmaker

func singleManifestForTest[T any](name string, opts ...ConfigOption) ([]Variable, error) {
	options := []DescribeOption{IncludeDefaults()}
	for _, o := range opts {
		options = append(options, o)
	}
	r, err := Manifest[T](name, options...)
	if err != nil {
		return nil, err
	}
	return r.Configs[0].Variables, nil
}
func loaderManifestForTest(l *Loader) ([]ConfigManifest, error) {
	r, err := l.Manifest(IncludeDefaults())
	return r.Configs, err
}

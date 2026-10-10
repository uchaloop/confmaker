module github.com/uchaloop/confmaker/examples/engines

go 1.27.0

require (
	github.com/caarlos0/env/v11 v11.4.1
	github.com/knadh/koanf/parsers/toml/v2 v2.2.2
	github.com/knadh/koanf/providers/rawbytes v1.0.1
	github.com/knadh/koanf/v2 v2.3.8
	github.com/pelletier/go-toml/v2 v2.4.3
	github.com/spf13/viper v1.21.0
	github.com/uchaloop/confmaker/v2 v2.0.0
	go.yaml.in/yaml/v3 v3.0.5
)

require (
	github.com/fsnotify/fsnotify v1.9.0 // indirect
	github.com/go-viper/mapstructure/v2 v2.4.0 // indirect
	github.com/knadh/koanf/maps v0.1.2 // indirect
	github.com/mitchellh/copystructure v1.2.0 // indirect
	github.com/mitchellh/reflectwalk v1.0.2 // indirect
	github.com/sagikazarmark/locafero v0.11.0 // indirect
	github.com/sourcegraph/conc v0.3.1-0.20240121214520-5f936abd7ae8 // indirect
	github.com/spf13/afero v1.15.0 // indirect
	github.com/spf13/cast v1.10.0 // indirect
	github.com/spf13/pflag v1.0.10 // indirect
	github.com/subosito/gotenv v1.6.0 // indirect
	golang.org/x/sys v0.29.0 // indirect
	golang.org/x/text v0.28.0 // indirect
)

// Run examples against the checked-out core.
replace github.com/uchaloop/confmaker/v2 => ../..

package confmaker

import (
	"strings"
	"testing"
)

type explicitNamedConfig struct {
	Host string `env:"HOST,notEmpty"`
}

func TestExplicitNameAndPrefix(t *testing.T) {
	t.Parallel()

	env := map[string]string{
		"CONFXDEFAULT_HOST":          "primary",
		"CONFXREPLICA_POSTGRES_HOST": "replica",
		"CONFXCUSTOM_HOST":           "custom",
	}

	loader := MakeLoader(WithEnv(env))
	primary := loader.Register[explicitNamedConfig]("confxdefault")
	replica := loader.Register[explicitNamedConfig]("replica", WithPrefix("CONFXREPLICA_POSTGRES_"))
	custom := loader.Register[explicitNamedConfig]("confxcustom")
	if err := loader.Load(); err != nil {
		t.Fatal(err)
	}

	for handle, want := range map[*Handle[explicitNamedConfig]]string{primary: "primary", replica: "replica", custom: "custom"} {
		if cfg, err := handle.Value(); err != nil || cfg.Host != want {
			t.Errorf("got %+v, %v, want host %q", cfg, err, want)
		}
	}

	variables, err := Manifest[explicitNamedConfig]("confxdefault")
	if err != nil || variables[0].Name != "CONFXDEFAULT_HOST" {
		t.Fatalf("manifest: %v, %v", variables, err)
	}

	variables, err = Manifest[explicitNamedConfig]("confxcustom")
	if err != nil || variables[0].Name != "CONFXCUSTOM_HOST" {
		t.Fatalf("manifest: %v, %v", variables, err)
	}
}

func TestMissingAndInvalidNames(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		add  func(*Loader)
		want string
	}{
		{"empty name", func(l *Loader) { l.Register[explicitNamedConfig]("") }, "instance name"},
		{"invalid name", func(l *Loader) { l.Register[explicitNamedConfig]("UPPER") }, "instance name"},
		{"empty prefix", func(l *Loader) { l.Register[explicitNamedConfig]("explicit", WithPrefix("")) }, "environment prefix"},
		{"nil option", func(l *Loader) { l.Register[explicitNamedConfig]("app", nil) }, "option must not be nil"},
		{"pointer type", func(l *Loader) { l.Register[*explicitNamedConfig]("") }, "must be a struct"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			loader := MakeLoader(WithEnv(nil))
			tc.add(loader)
			err := loader.Load()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want %q", err, tc.want)
			}
		})
	}
}

func TestLoaderRejectsRepeatedNameAcrossTypes(t *testing.T) {
	t.Parallel()

	for _, prefix := range []string{"CONFXDUPLICATE_", "CONFXOTHER_"} {
		loader := MakeLoader(WithEnv(nil))
		loader.Register[struct {
			Host string `env:"HOST"`
		}]("confxduplicate")
		loader.Register[struct {
			Port int `env:"PORT"`
		}]("confxduplicate", WithPrefix(prefix))
		err := loader.Load()
		if err == nil || !strings.Contains(err.Error(), "registered more than once") {
			t.Fatalf("prefix %s: %v", prefix, err)
		}
	}

	descriptors := []descriptor{
		{instanceName: "same", fields: []fieldSpec{{Name: "CONFX_SHARED"}}},
		{instanceName: "same", fields: []fieldSpec{{Name: "CONFX_SHARED"}}},
	}
	if _, err := claimedVariables(descriptors); err == nil {
		t.Fatal("same-label variable collision accepted")
	}
}

// Invalid types must fail before name/prefix validation, but after malformed
// options. All public entry points must report the same registration error.
func TestRegistrationErrorPrecedence(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name         string
		instanceName string
		opts         []ConfigOption
		want         string
	}{
		{"nil option", "app", []ConfigOption{nil}, "a config option must not be nil"},
		{"empty name", "", nil, "a config must be a struct, got int"},
		{"invalid name", "INVALID", nil, "a config must be a struct, got int"},
		{"invalid prefix", "valid", []ConfigOption{WithPrefix("")}, "a config must be a struct, got int"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, manifestErr := Manifest[int](tc.instanceName, tc.opts...)
			loader := MakeLoader(WithEnv(nil))
			loader.Register[int](tc.instanceName, tc.opts...)
			loaderErr := loader.Load()

			loadOpts := []LoadOption{WithEnv(nil)}
			for _, opt := range tc.opts {
				loadOpts = append(loadOpts, opt)
			}

			_, loadErr := Load[int](tc.instanceName, loadOpts...)

			for entry, err := range map[string]error{"Manifest": manifestErr, "Loader": loaderErr} {
				if err == nil || err.Error() != tc.want {
					t.Errorf("%s: got %v, want %q", entry, err, tc.want)
				}
			}
			// Load interprets an untyped nil as an environment option and
			// aggregates that error with the invalid registration.
			wantLoad := tc.want
			if tc.name == "nil option" {
				wantLoad = "a load option must not be nil\na config must be a struct, got int"
			}

			if loadErr == nil || loadErr.Error() != wantLoad {
				t.Errorf("Load: got %v, want %q", loadErr, wantLoad)
			}
		})
	}
}

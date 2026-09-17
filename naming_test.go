package confmaker

import (
	"strings"
	"testing"
)

type defaultNamedConfig struct {
	Host string `env:"HOST,notEmpty"`
}

func (defaultNamedConfig) ConfigName() string { return "confxdefault" }

type pointerNamedConfig struct {
	Host string `env:"HOST"`
}

func (c *pointerNamedConfig) ConfigName() string {
	if c == nil || len(c.Host) != 0 {
		panic("ConfigName must receive a fresh zero value")
	}

	return "confxpointer"
}
func (*pointerNamedConfig) SetDefaults() { panic("name resolution must not run defaults") }

type invalidNamedConfig struct {
	Host string `env:"HOST"`
}

func (invalidNamedConfig) ConfigName() string { return "Invalid" }

type panicNamedConfig struct {
	Host string `env:"HOST"`
}

func (panicNamedConfig) ConfigName() string { panic("an explicit name must bypass ConfigName") }

func TestDefaultNameAndOverride(t *testing.T) {
	t.Parallel()

	env := map[string]string{
		"CONFXDEFAULT_HOST":          "primary",
		"CONFXREPLICA_POSTGRES_HOST": "replica",
		"CONFXCUSTOM_HOST":           "custom",
	}

	loader := MakeLoader(WithEnv(env))
	primary := loader.Add[defaultNamedConfig]()
	replica := loader.Add[defaultNamedConfig](WithName("replica"), WithPrefix("CONFXREPLICA_POSTGRES_"))
	custom := loader.Add[defaultNamedConfig](WithName("confxcustom"))
	if err := loader.Load(); err != nil {
		t.Fatal(err)
	}

	for handle, want := range map[*Handle[defaultNamedConfig]]string{primary: "primary", replica: "replica", custom: "custom"} {
		if cfg, err := handle.Value(); err != nil || cfg.Host != want {
			t.Errorf("got %+v, %v, want host %q", cfg, err, want)
		}
	}

	variables, err := Manifest[defaultNamedConfig]()
	if err != nil || variables[0].Name != "CONFXDEFAULT_HOST" {
		t.Fatalf("manifest: %v, %v", variables, err)
	}

	variables, err = Manifest[defaultNamedConfig](WithName("confxcustom"))
	if err != nil || variables[0].Name != "CONFXCUSTOM_HOST" {
		t.Fatalf("manifest: %v, %v", variables, err)
	}
}

func TestNameResolution(t *testing.T) {
	set, err := resolveSettings[pointerNamedConfig](nil)
	if err != nil || set.name != "confxpointer" {
		t.Fatalf("pointer receiver: %+v, %v", set, err)
	}

	set, err = resolveSettings[panicNamedConfig]([]ConfigOption{WithName("explicit"), WithPrefix("CUSTOM_")})
	if err != nil || set.name != "explicit" || set.prefix != "CUSTOM_" {
		t.Fatalf("explicit: %+v, %v", set, err)
	}

	set, err = resolveSettings[panicNamedConfig]([]ConfigOption{WithName("replica")})
	if err != nil || set.prefix != "REPLICA_" {
		t.Fatalf("named: %+v, %v", set, err)
	}

	set, err = resolveSettings[defaultNamedConfig]([]ConfigOption{WithPrefix("CUSTOM_")})
	if err != nil || set.name != "confxdefault" || set.prefix != "CUSTOM_" {
		t.Fatalf("prefix: %+v, %v", set, err)
	}
}

func TestMissingAndInvalidNames(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		add  func(*Loader)
		want string
	}{
		{"missing", func(l *Loader) { l.Add[strictConfig]() }, "WithName or implement ConfigName"},
		{"invalid method", func(l *Loader) { l.Add[invalidNamedConfig]() }, "instance name"},
		{"empty override", func(l *Loader) { l.Add[defaultNamedConfig](WithName("")) }, "instance name"},
		{"invalid override", func(l *Loader) { l.Add[defaultNamedConfig](WithName("UPPER")) }, "instance name"},
		{"empty prefix", func(l *Loader) { l.Add[defaultNamedConfig](WithPrefix("")) }, "environment prefix"},
		{"nil option", func(l *Loader) { l.Add[defaultNamedConfig](nil) }, "option must not be nil"},
		{"repeated name", func(l *Loader) { l.Add[defaultNamedConfig](WithName("replica"), WithName("other")) }, "instance name more than once"},
		{"pointer type", func(l *Loader) { l.Add[*defaultNamedConfig]() }, "must be a struct"},
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

	if _, err := Manifest[strictConfig](); err == nil || !strings.Contains(err.Error(), "ConfigName") {
		t.Fatalf("missing manifest name: %v", err)
	}

	if _, err := Manifest[invalidNamedConfig](); err == nil {
		t.Fatal("invalid default name accepted")
	}

	if _, err := Manifest[strictConfig](WithName("explicit")); err != nil {
		t.Fatal(err)
	}

	if _, err := Manifest[panicNamedConfig](WithName("explicit")); err != nil {
		t.Fatal(err)
	}
}

func TestLoaderRejectsRepeatedNameAcrossTypes(t *testing.T) {
	t.Parallel()

	type first struct {
		Host string `env:"HOST"`
	}

	type second struct {
		Port int `env:"PORT"`
	}

	for _, prefix := range []string{"CONFXDUPLICATE_", "CONFXOTHER_"} {
		loader := MakeLoader(WithEnv(nil))
		loader.Add[first](WithName("confxduplicate"))
		loader.Add[second](WithName("confxduplicate"), WithPrefix(prefix))
		err := loader.Load()
		if err == nil || !strings.Contains(err.Error(), "registered more than once") {
			t.Fatalf("prefix %s: %v", prefix, err)
		}
	}

	descriptors := []descriptor{
		{label: "same", fields: []fieldSpec{{Name: "CONFX_SHARED"}}},
		{label: "same", fields: []fieldSpec{{Name: "CONFX_SHARED"}}},
	}
	if _, err := claimedVariables(descriptors); err == nil {
		t.Fatal("same-label variable collision accepted")
	}
}

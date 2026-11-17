package confmaker

import (
	"context"
	"errors"
	"testing"
)

type prefixConfig struct {
	Host string `env:"HOST,notEmpty"`
	TLS  struct {
		Mode string `env:"MODE"`
	} `envPrefix:"TLS_"`
}

func (prefixConfig) EnvPrefix() string { return "SERVICE_" }

type pointerPrefixConfig struct {
	Host string `env:"HOST"`
}

func (*pointerPrefixConfig) EnvPrefix() string { return "STORE_" }

type panicPrefixConfig struct{}

func (panicPrefixConfig) EnvPrefix() string { panic("prefix callback") }

type invalidPrefixConfig struct{}

func (invalidPrefixConfig) EnvPrefix() string { return "invalid" }

func TestTypePrefix(t *testing.T) {
	for _, tc := range []struct {
		name, prefix, identity string
		opts                   []ConfigOption
	}{
		{name: "", prefix: "SERVICE_", identity: "service"},
		{name: "service", prefix: "SERVICE_", identity: "service"},
		{name: "master", prefix: "MASTER_SERVICE_", identity: "master"},
		{name: "read-replica", prefix: "READ_REPLICA_SERVICE_", identity: "read-replica"},
		{name: "master_service", prefix: "MASTER_SERVICE_SERVICE_", identity: "master_service"},
		{name: "master", prefix: "MASTER_", identity: "master", opts: []ConfigOption{WithPrefix("MASTER_")}},
		{name: "", prefix: "CUSTOM_", identity: "service", opts: []ConfigOption{WithPrefix("CUSTOM_")}},
	} {
		t.Run(tc.prefix+tc.name, func(t *testing.T) {
			l := MakeLoader(WithEnv(map[string]string{tc.prefix + "HOST": "host", tc.prefix + "TLS_MODE": "require"}))
			h := l.Register[prefixConfig](tc.name, tc.opts...)
			if h.Name() != tc.identity {
				t.Fatalf("identity = %q", h.Name())
			}
			m, err := l.Manifest()
			if err != nil {
				t.Fatal(err)
			}
			if len(m.Configs) != 1 || m.Configs[0].Prefix != tc.prefix || m.Configs[0].InstanceName != tc.identity {
				t.Fatalf("manifest: %+v", m)
			}
			if err := l.Load(); err != nil {
				t.Fatal(err)
			}
			cfg, err := h.Value()
			if err != nil || cfg.Host != "host" || cfg.TLS.Mode != "require" {
				t.Fatalf("value: %+v, %v", cfg, err)
			}
			cfg, err = Load[prefixConfig](tc.name, WithEnv(map[string]string{tc.prefix + "HOST": "host"}), WithPrefix(tc.prefix))
			if err != nil || cfg.Host != "host" {
				t.Fatalf("Load: %+v %v", cfg, err)
			}
		})
	}
}

func TestPointerTypePrefix(t *testing.T) {
	cfg, err := Load[pointerPrefixConfig]("", WithEnv(map[string]string{"STORE_HOST": "value"}))
	if err != nil || cfg.Host != "value" {
		t.Fatalf("value: %+v %v", cfg, err)
	}
}

func TestTypePrefixConflictsAndUnknownVariables(t *testing.T) {
	for _, name := range []string{"duplicate", "unknown", "invalid"} {
		t.Run(name, func(t *testing.T) {
			l := MakeLoader(WithEnv(map[string]string{"SERVICE_HOST": "host"}))
			switch name {
			case "duplicate":
				l.Register[prefixConfig]("")
				l.Register[prefixConfig]("service")
			case "unknown":
				l = MakeLoader(WithEnv(map[string]string{"SERVICE_HOST": "host", "SERVICE_TYPO": "value"}))
				l.Register[prefixConfig]("")
			case "invalid":
				l.Register[invalidPrefixConfig]("app")
			}
			if err := l.Load(); err == nil {
				t.Fatal("expected registration or load error")
			}
		})
	}
}

func TestTypePrefixCallbackBoundaries(t *testing.T) {
	t.Run("panic propagates", func(t *testing.T) {
		l := MakeLoader(WithEnv(nil))
		func() {
			defer func() {
				if recover() != "prefix callback" {
					t.Error("panic changed")
				}
			}()
			l.Register[panicPrefixConfig]("x")
		}()
		if err := l.Load(); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("explicit override", func(t *testing.T) {
		_, err := Load[panicPrefixConfig]("x", WithPrefix("X_"), WithEnv(nil))
		if err != nil {
			t.Fatal(err)
		}
	})
	t.Run("late registration", func(t *testing.T) {
		l := MakeLoader(WithEnv(nil))
		if err := l.Load(); err != nil {
			t.Fatal(err)
		}
		h := l.Register[panicPrefixConfig]("")
		if _, err := h.Value(); !errors.Is(err, ErrRegisteredAfterLoad) {
			t.Fatal(err)
		}
	})
	t.Run("custom engine", func(t *testing.T) {
		l := MakeLoader(WithEngine(EngineFunc(func(context.Context, LoadRequest) error { return nil })))
		l.Register[panicPrefixConfig]("custom")
		if err := l.Load(); err != nil {
			t.Fatal(err)
		}
		l = MakeLoader(WithEngine(EngineFunc(func(context.Context, LoadRequest) error { t.Fatal("engine called"); return nil })))
		l.Register[panicPrefixConfig]("")
		if err := l.Load(); err == nil {
			t.Fatal("empty engine name accepted")
		}
	})
}

func TestDefaultPrefixDiagnosticsAndManifest(t *testing.T) {
	l := MakeLoader(WithEnv(map[string]string{"SERVICE_HOST": "host"}), WithDiagnostics())
	l.Register[prefixConfig]("")
	if err := l.Load(); err != nil {
		t.Fatal(err)
	}
	report := l.Report()
	if len(report.Configs) != 1 || report.Configs[0].InstanceName != "service" {
		t.Fatalf("report: %+v", report)
	}
	manifest, err := Manifest[prefixConfig]("")
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Configs[0].InstanceName != "service" || manifest.Configs[0].Variables[0].Name != "SERVICE_HOST" {
		t.Fatalf("manifest: %+v", manifest)
	}
}

type invalidSchemaPrefixConfig struct {
	Host string `env:"HOST,unsupported"`
}

func (invalidSchemaPrefixConfig) EnvPrefix() string { return "SERVICE_" }

func TestDefaultPrefixInvalidSchemaIdentity(t *testing.T) {
	l := MakeLoader(WithEnv(nil), WithDiagnostics())
	h := l.Register[invalidSchemaPrefixConfig]("")
	if err := l.Load(); err == nil {
		t.Fatal("invalid schema accepted")
	}
	report := l.Report()
	if h.Name() != "service" || report.Configs[0].InstanceName != "service" || report.Configs[0].Prefix != "SERVICE_" {
		t.Fatalf("identity lost: %q %+v", h.Name(), report)
	}
}

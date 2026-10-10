package confmaker

import (
	"errors"
	"reflect"
	"sync"
	"testing"
)

type manifestOnlyConfig struct {
	Host string `env:"HOST,notEmpty"`
}

func (*manifestOnlyConfig) Validate() error { panic("Manifest called Validate") }

type manifestPanicDefaults struct {
	Host string `env:"HOST"`
}

func (*manifestPanicDefaults) SetDefaults() { panic("invalid manifest evaluated defaults") }

func TestLoaderManifestMatchesSingleConfig(t *testing.T) {
	t.Parallel()
	loader := MakeLoader()
	loader.Register[manifestConfig]("replica", WithPrefix("CUSTOM_"))
	loader.Register[manifestConfig]("primary")
	got, err := loaderManifestForTest(loader)
	if err != nil {
		t.Fatal(err)
	}

	primary, err := singleManifestForTest[manifestConfig]("primary")
	if err != nil {
		t.Fatal(err)
	}

	replica, err := singleManifestForTest[manifestConfig]("replica", WithPrefix("CUSTOM_"))
	if err != nil {
		t.Fatal(err)
	}

	want := []ConfigManifest{
		{InstanceName: "primary", Prefix: "PRIMARY_", Variables: primary},
		{InstanceName: "replica", Prefix: "CUSTOM_", Variables: replica},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestLoaderManifestDoesNotUseLoadOptionsOrValidate(t *testing.T) {
	t.Parallel()
	loader := MakeLoader(WithEnv(map[string]string{"APP_HOST": "env", "APP_TYPO": "x"}))
	handle := loader.Register[manifestOnlyConfig]("app")
	got, err := loaderManifestForTest(loader)
	if err != nil || len(got) != 1 || len(got[0].Variables[0].Default.Text) != 0 {
		t.Fatalf("manifest: %v, %v", got, err)
	}

	invalidOptions := MakeLoader(nil, WithEnv(nil), WithEnv(nil))
	invalidOptions.Register[manifestOnlyConfig]("app")
	if _, err := loaderManifestForTest(invalidOptions); err != nil {
		t.Fatalf("load options affected manifest: %v", err)
	}

	if _, err := handle.Value(); !errors.Is(err, ErrNotLoaded) {
		t.Fatalf("handle: %v", err)
	}
}

func TestLoaderManifestKeepsDefaultsSeparateFromLoadedValues(t *testing.T) {
	t.Parallel()
	loader := MakeLoader(WithEnv(map[string]string{"APP_ITEMS": "loaded"}))
	handle := loader.Register[schemaNested]("app")
	before, err := loaderManifestForTest(loader)
	if err != nil {
		t.Fatal(err)
	}
	// Manifest leaves registration open.
	loader.Register[struct{}]("empty")
	if err := loader.Load(); err != nil {
		t.Fatal(err)
	}

	cfg, err := handle.Value()
	if err != nil || len(cfg.Items) != 1 || cfg.Items[0] != "loaded" {
		t.Fatalf("loaded: %+v, %v", cfg, err)
	}

	cfg.Items[0] = "mutated"
	after, err := loaderManifestForTest(loader)
	if err != nil || len(after) != 2 || !reflect.DeepEqual(before[0], after[0]) {
		t.Fatalf("manifest changed: %v, %v", after, err)
	}

	after[0].Variables[0].Name = "MUTATED"
	again, err := loaderManifestForTest(loader)
	if err != nil || !reflect.DeepEqual(before[0], again[0]) {
		t.Fatalf("manifest shares result storage: %v, %v", again, err)
	}

	cfg, err = handle.Value()
	if err != nil || cfg.Items[0] != "mutated" {
		t.Fatalf("manifest changed loaded config: %+v, %v", cfg, err)
	}
}

func TestLoaderManifestRejectsDeclarationsAndConflictsBeforeDefaults(t *testing.T) {
	t.Parallel()
	cases := map[string]func(*Loader){
		"declaration": func(l *Loader) { l.Register[invalidDefaults]("invalid") },
		"name":        func(l *Loader) { l.Register[manifestPanicDefaults]("guard", WithPrefix("OTHER_")) },
		"prefix":      func(l *Loader) { l.Register[manifestPanicDefaults]("other", WithPrefix("GUARD_")) },
		"variable": func(l *Loader) {
			l.Register[struct {
				Host string `env:"MAIN_HOST"`
			}]("db")
			l.Register[struct {
				Host string `env:"HOST"`
			}]("db_main")
		},
	}

	for name, register := range cases {
		t.Run(name, func(t *testing.T) {
			loader := MakeLoader()
			loader.Register[manifestPanicDefaults]("guard")
			register(loader)
			got, err := loaderManifestForTest(loader)
			if err == nil || got != nil {
				t.Fatalf("got %v, %v", got, err)
			}
		})
	}
}

func TestLoaderManifestReportsAllRenderErrorsAndDoesNotPreventLoad(t *testing.T) {
	l := MakeLoader(WithEnv(nil))
	l.Register[struct{}]("good")
	l.Register[brokenTextConfig]("z")
	l.Register[brokenTextConfig]("a")
	r, err := l.Manifest(IncludeDefaults())
	if err != nil || len(r.Configs) != 3 || len(r.Problems) != 2 || r.Problems[0].InstanceName != "a" || r.Problems[1].InstanceName != "z" {
		t.Fatalf("%+v %v", r, err)
	}
	if err := l.Load(); err != nil {
		t.Fatal(err)
	}
}

func TestLoaderManifestAfterFailedLoad(t *testing.T) {
	t.Parallel()
	loader := MakeLoader(WithEnv(nil))
	handle := loader.Register[manifestOnlyConfig]("app")
	loadErr := loader.Load()
	if loadErr == nil {
		t.Fatal("expected missing required variable")
	}

	got, err := loaderManifestForTest(loader)
	if err != nil || len(got) != 1 {
		t.Fatalf("manifest: %v, %v", got, err)
	}

	if _, err := handle.Value(); err != loadErr {
		t.Fatalf("load result changed: %v", err)
	}
}

func TestEmptyLoaderManifest(t *testing.T) {
	t.Parallel()
	var loader Loader
	got, err := loaderManifestForTest(&loader)
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("got %v, %v", got, err)
	}
}

// This hook is used only by the non-parallel snapshot test below.
var manifestDefaultsHook func()

type manifestSnapshotConfig struct {
	Host string `env:"HOST"`
}

func (*manifestSnapshotConfig) SetDefaults() { manifestDefaultsHook() }

func TestLoaderManifestSnapshotsBeforeCallingUserCode(t *testing.T) {
	withinDeadline(t, func() {
		loader := MakeLoader(WithEnv(nil))
		handle := loader.Register[manifestSnapshotConfig]("first")
		entered, resume := make(chan struct{}), make(chan struct{})
		var once sync.Once
		manifestDefaultsHook = func() { once.Do(func() { close(entered); <-resume }) }
		defer func() { manifestDefaultsHook = nil }()
		var got []ConfigManifest
		var manifestErr error
		done := make(chan struct{})
		go func() { defer close(done); got, manifestErr = loaderManifestForTest(loader) }()
		<-entered
		loader.Register[struct{}]("second")
		if _, err := handle.Value(); !errors.Is(err, ErrNotLoaded) {
			t.Fatalf("handle: %v", err)
		}

		close(resume)
		<-done
		if manifestErr != nil || len(got) != 1 {
			t.Fatalf("snapshot: %v, %v", got, manifestErr)
		}

		var group sync.WaitGroup
		for range 8 {
			group.Go(func() {
				entries, err := loaderManifestForTest(loader)
				if err != nil || len(entries) != 2 {
					t.Errorf("concurrent manifest: %v, %v", entries, err)
				}
			})
		}

		group.Go(func() {
			if err := loader.Load(); err != nil {
				t.Errorf("load: %v", err)
			}
		})
		group.Wait()
	})
}

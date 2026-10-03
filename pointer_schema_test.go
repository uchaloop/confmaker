package confmaker

import (
	"testing"
	"time"
)

type recursivePointer *recursivePointer
type pointerCycleConfig struct {
	Value recursivePointer `env:"VALUE"`
}
type pointerCycleSliceConfig struct {
	Values []recursivePointer `env:"VALUES"`
}
type pointerCycleMapConfig struct {
	Values map[string]recursivePointer `env:"VALUES"`
}
type nestedRequiredConfig struct {
	Host string `env:"HOST,notEmpty"`
}

func checkPointerDeclaration[T any](t *testing.T) {
	t.Helper()

	variables, err := Manifest[T]("app")
	problems := ConfigErrors(err)
	if variables != nil || len(problems) == 0 || problems[0].Kind != ErrorDeclaration || problems[0].InstanceName != "app" {
		t.Fatalf("expected declaration error, got %v, %v", variables, err)
	}

	loader := MakeLoader(WithEnv(nil))
	loader.Register[T]("app")
	if err := loader.Load(); len(ConfigErrors(err)) == 0 {
		t.Fatalf("load accepted declaration: %v", err)
	}
}

func TestPointerSchemaRestrictions(t *testing.T) {
	checkPointerDeclaration[pointerCycleConfig](t)
	checkPointerDeclaration[pointerCycleSliceConfig](t)
	checkPointerDeclaration[pointerCycleMapConfig](t)
	checkPointerDeclaration[struct{ DB **nestedRequiredConfig }](t)
	checkPointerDeclaration[struct{ DB []*nestedRequiredConfig }](t)
	checkPointerDeclaration[struct {
		DB map[string]**nestedRequiredConfig
	}](t)
	checkPointerDeclaration[struct {
		Values map[*string]int `env:"VALUES"`
	}](t)
}

func TestScalarPointersStillLoad(t *testing.T) {
	cfg, err := Load[struct {
		Timeout **time.Duration `env:"TIMEOUT"`
	}]("app", WithEnv(map[string]string{"APP_TIMEOUT": "3s"}))
	if err != nil || cfg.Timeout == nil || *cfg.Timeout == nil || **cfg.Timeout != 3*time.Second {
		t.Fatalf("scalar pointers: %+v, %v", cfg, err)
	}
}

func TestRecursiveJSONStructIsRejected(t *testing.T) {
	type node struct {
		Value int
		Next  *node
	}
	checkPointerDeclaration[struct {
		Root node `env:"ROOT" envFormat:"json"`
	}](t)
}

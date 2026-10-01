package confmaker

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
)

func TestStructuredLoadErrors(t *testing.T) {
	type config struct {
		Required string `env:"REQUIRED,required"`
		Empty    string `env:"EMPTY,notEmpty"`
		Pool     struct {
			Size int `env:"SIZE"`
		} `envPrefix:"POOL_"`
	}
	_, err := Load[config]("app", WithEnv(map[string]string{
		"APP_EMPTY": "", "APP_POOL_SIZE": "bad", "APP_UNKNOWN": "ignored-value",
	}))
	problems := ConfigErrors(fmt.Errorf("startup: %w", err))
	wantKinds := []ErrorKind{ErrorUnknownVariable, ErrorRequired, ErrorEmpty, ErrorParse}
	if len(problems) != len(wantKinds) {
		t.Fatalf("problems: %#v; %v", problems, err)
	}
	for i, kind := range wantKinds {
		if problems[i].Kind != kind {
			t.Fatalf("problem %d: %+v", i, problems[i])
		}
	}
	for _, problem := range problems[1:] {
		if problem.InstanceName != "app" || len(problem.VariableName) == 0 || len(problem.FieldPath) == 0 {
			t.Fatalf("missing context: %#v", problem)
		}
	}
	if problems[3].FieldPath != "Pool.Size" || problems[3].VariableName != "APP_POOL_SIZE" {
		t.Fatalf("nested context: %#v", problems[3])
	}
	var first *ConfigError
	if !errors.As(err, &first) || first != problems[0] {
		t.Fatal("errors.As does not find first problem")
	}
}

var validationCause = errors.New("invalid pool size")

type structuredValidationConfig struct{}

func (structuredValidationConfig) Validate() error { return fmt.Errorf("pool: %w", validationCause) }

func TestStructuredValidationCause(t *testing.T) {
	_, err := Load[structuredValidationConfig]("app", WithEnv(nil))
	problems := ConfigErrors(err)
	if len(problems) != 1 || problems[0].Kind != ErrorValidation || problems[0].InstanceName != "app" {
		t.Fatalf("validation: %v, %#v", err, problems)
	}
	if !errors.Is(err, validationCause) {
		t.Fatal("validation cause lost")
	}
	if len(problems[0].VariableName) != 0 || len(problems[0].FieldPath) != 0 {
		t.Fatal("invented validation field")
	}
}

func TestStructuredDeclarationsAndConflicts(t *testing.T) {
	type invalid struct {
		One string `env:"ONE,unsupported"`
		Two string `env:"TWO" envPrefix:"BAD_"`
	}
	_, err := Manifest[invalid]("app")
	problems := ConfigErrors(err)
	if len(problems) != 2 {
		t.Fatalf("schema: %v", err)
	}
	for i, field := range []string{"One", "Two"} {
		if problems[i].Kind != ErrorDeclaration || problems[i].FieldPath != field || problems[i].InstanceName != "app" || len(problems[i].VariableName) == 0 {
			t.Fatalf("schema context: %#v", problems[i])
		}
	}
	type collision struct {
		One string `env:"VALUE"`
		Two string `env:"VALUE"`
	}
	_, err = Manifest[collision]("app")
	problems = ConfigErrors(err)
	if len(problems) != 1 || problems[0].Kind != ErrorConflict || problems[0].VariableName != "APP_VALUE" {
		t.Fatalf("collision: %v", err)
	}
	loader := MakeLoader()
	loader.Register[struct{}]("app")
	loader.Register[struct{}]("app")
	_, err = loader.Manifest()
	problems = ConfigErrors(err)
	if len(problems) != 2 {
		t.Fatalf("registration conflicts: %v", err)
	}
	for _, problem := range problems {
		if problem.Kind != ErrorConflict {
			t.Fatalf("conflict: %#v", problem)
		}
	}
	_, err = Manifest[int]("app")
	problems = ConfigErrors(err)
	if len(problems) != 1 || problems[0].Kind != ErrorDeclaration || problems[0].InstanceName != "app" {
		t.Fatalf("type: %v", err)
	}
}

func TestConfigErrorsTraversal(t *testing.T) {
	first := &ConfigError{Kind: ErrorRequired}
	second := &ConfigError{Kind: ErrorEmpty}
	tree := fmt.Errorf("outer: %w", errors.Join(first, fmt.Errorf("nested: %w", second), first))
	if got := ConfigErrors(tree); !reflect.DeepEqual(got, []*ConfigError{first, second, first}) {
		t.Fatalf("order: %#v", got)
	}
	if ConfigErrors(nil) != nil || ConfigErrors(errors.New("ordinary")) != nil {
		t.Fatal("unexpected problems")
	}
}

func TestStructuredParseCauseAndSecret(t *testing.T) {
	cause := errors.New("private value")
	field := fieldSpec{Name: "APP_TOKEN", Type: "token", field: "Token"}
	err := wrapConfigError("app", describeParseError(field, cause))
	if !errors.Is(err, cause) {
		t.Fatal("ordinary parse cause lost")
	}
	field.Secret = true
	err = wrapConfigError("app", describeParseError(field, cause))
	if errors.Is(err, cause) {
		t.Fatal("secret parse cause exposed")
	}
	problems := ConfigErrors(err)
	if len(problems) != 1 || problems[0].Kind != ErrorParse || problems[0].InstanceName != "app" {
		t.Fatalf("secret context: %v", err)
	}
	for current := err; current != nil; current = errors.Unwrap(current) {
		if current.Error() == cause.Error() {
			t.Fatal("secret in error chain")
		}
	}
}

type failingDefaultText string

var defaultRenderCause = errors.New("cannot render default")

func (*failingDefaultText) UnmarshalText([]byte) error  { return nil }
func (failingDefaultText) MarshalText() ([]byte, error) { return nil, defaultRenderCause }

func TestStructuredDefaultRender(t *testing.T) {
	type config struct {
		Value failingDefaultText `env:"VALUE"`
	}
	_, err := Manifest[config]("app")
	problems := ConfigErrors(err)
	if len(problems) != 1 || problems[0].Kind != ErrorDefaultRender || problems[0].FieldPath != "Value" || problems[0].InstanceName != "app" {
		t.Fatalf("render: %v", err)
	}
	if !errors.Is(err, defaultRenderCause) {
		t.Fatal("render cause lost")
	}
}

func ExampleConfigErrors() {
	type config struct {
		URL string `env:"URL,notEmpty"`
	}
	_, err := Load[config]("api", WithEnv(map[string]string{}))
	for _, problem := range ConfigErrors(err) {
		fmt.Println(problem.Kind, problem.InstanceName, problem.VariableName, problem.FieldPath)
	}
	// Output: required api API_URL URL
}

func TestStructuredLoadOptions(t *testing.T) {
	loader := MakeLoader(nil, WithEnv(map[string]string{}), WithEnv(map[string]string{}))
	problems := ConfigErrors(loader.Load())
	if len(problems) != 2 {
		t.Fatalf("options: %#v", problems)
	}
	for _, problem := range problems {
		if problem.Kind != ErrorDeclaration || len(problem.InstanceName) != 0 {
			t.Fatalf("option context: %#v", problem)
		}
	}
}

func TestUnknownVariableOwnership(t *testing.T) {
	for _, overlap := range []bool{false, true} {
		loader := MakeLoader(WithEnv(map[string]string{"APP_POOL_UNKNOWN": "value"}))
		loader.Register[struct{}]("app")
		if overlap {
			loader.Register[struct{}]("app_pool")
		}
		problems := ConfigErrors(loader.Load())
		if len(problems) != 1 {
			t.Fatalf("unknown: %#v", problems)
		}
		want := "app"
		if overlap {
			want = ""
		}
		if problems[0].InstanceName != want {
			t.Fatalf("ownership: %#v", problems[0])
		}
	}
}

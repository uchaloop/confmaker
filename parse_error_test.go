package confmaker

import (
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestScalarParseErrorsPreserveCause(t *testing.T) {
	for _, test := range []struct {
		name    string
		target  reflect.Type
		raw     string
		cause   error
		message string
	}{
		{"bool", reflect.TypeFor[bool](), "bad", strconv.ErrSyntax, `"bad" is not a boolean`},
		{"int syntax", reflect.TypeFor[int8](), "bad", strconv.ErrSyntax, `"bad" is not int8`},
		{"int range", reflect.TypeFor[int8](), "128", strconv.ErrRange, `"128" is not int8`},
		{"uint syntax", reflect.TypeFor[uint8](), "-1", strconv.ErrSyntax, `"-1" is not uint8`},
		{"uint range", reflect.TypeFor[uint8](), "256", strconv.ErrRange, `"256" is not uint8`},
		{"float syntax", reflect.TypeFor[float32](), "bad", strconv.ErrSyntax, `"bad" is not float32`},
		{"float range", reflect.TypeFor[float32](), "1e100", strconv.ErrRange, `"1e100" is not float32`},
	} {
		t.Run(test.name, func(t *testing.T) {
			parse, err := scalarParser(test.target)
			if err != nil {
				t.Fatal(err)
			}

			err = parse(reflect.New(test.target).Elem(), test.raw)
			var numberError *strconv.NumError
			if err == nil || err.Error() != test.message || !errors.Is(err, test.cause) || !errors.As(err, &numberError) || numberError.Num != test.raw {
				t.Fatalf("message or cause lost: %v", err)
			}

			secretError := describeParseError(fieldSpec{Name: "APP_SECRET", Type: test.target.String(), Secret: true}, err)
			if errors.As(secretError, &numberError) || errors.Is(secretError, test.cause) || strings.Contains(secretError.Error(), test.raw) {
				t.Fatalf("secret cause exposed: %v", secretError)
			}
		})
	}
}

func TestDurationParseErrorPreservesCause(t *testing.T) {
	err := parseDuration(reflect.New(reflect.TypeFor[time.Duration]()).Elem(), "bad")
	if err == nil || err.Error() != `"bad" is not a duration such as "30s" or "5m"` {
		t.Fatalf("message: %v", err)
	}

	cause := errors.Unwrap(err)
	_, expected := time.ParseDuration("bad")
	if cause == nil || cause.Error() != expected.Error() {
		t.Fatalf("cause: %v", cause)
	}
}

func TestLoadPreservesCollectionParseCauses(t *testing.T) {
	_, err := Load[struct {
		Values []int8           `env:"VALUES"`
		Limits map[string]uint8 `env:"LIMITS"`
	}]("app", WithEnv(map[string]string{"APP_VALUES": "128", "APP_LIMITS": "max:bad"}))
	problems := ConfigErrors(err)
	if len(problems) != 2 {
		t.Fatalf("problems: %v", err)
	}

	for i, cause := range []error{strconv.ErrRange, strconv.ErrSyntax} {
		var numberError *strconv.NumError
		if problems[i].Kind != ErrorParse || problems[i].InstanceName != "app" || !errors.Is(problems[i], cause) || !errors.As(problems[i], &numberError) {
			t.Fatalf("problem: %#v", problems[i])
		}
	}

	if !errors.Is(err, strconv.ErrRange) || !errors.Is(err, strconv.ErrSyntax) {
		t.Fatalf("joined causes: %v", err)
	}
}

package confmaker

import "reflect"

// DefaultState describes evaluation without claiming an explicit default exists.
type DefaultState string

const (
	DefaultNotEvaluated DefaultState = "not_evaluated"
	DefaultZero         DefaultState = "zero"
	DefaultRendered     DefaultState = "rendered"
	DefaultRedacted     DefaultState = "redacted"
	DefaultUnrenderable DefaultState = "unrenderable"
)

// DefaultInfo holds ENV text only when State is DefaultRendered.
type DefaultInfo struct {
	State DefaultState
	Text  string
}

func evaluateDefaults(d descriptor, variables []Variable) []ManifestProblem {
	root := reflect.ValueOf(d.defaults()).Elem()
	var problems []ManifestProblem
	for i, f := range d.fields {
		if f.Secret {
			continue
		}
		target := root.FieldByIndex(f.index)
		if target.IsZero() {
			variables[i].Default = DefaultInfo{State: DefaultZero}
			continue
		}
		text, err := f.render(target)
		if err != nil {
			variables[i].Default = DefaultInfo{State: DefaultUnrenderable}
			problems = append(problems, ManifestProblem{Kind: ErrorDefaultRender, InstanceName: d.instanceName, VariableName: f.Name, FieldPath: f.field})
		} else {
			variables[i].Default = DefaultInfo{State: DefaultRendered, Text: text}
		}
	}
	return problems
}
func setDefaults[T any](cfg *T) {
	if d, ok := any(cfg).(interface{ SetDefaults() }); ok {
		d.SetDefaults()
	}
}

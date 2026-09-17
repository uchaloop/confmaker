package confx

import (
	"fmt"
	"reflect"
	"slices"

	"github.com/uchaloop/confmaker"
	"github.com/uchaloop/utilfx"
	"go.uber.org/fx"
)

// registrarGroup is the value group that carries each Provide's registration to
// Module.
const registrarGroup = "confx_registrars"

// optionalRegistry lets a registration see that Module is missing and say so.
const optionalRegistry = `optional:"true"`

// registrar adds one Provide's config to an application's loader.
type registrar func(*registry)

// registry is one application's loader and the handles its Provide calls
// received. Provide options can be reused across applications, so handles are
// kept per application, keyed by the Provide call that made them.
type registry struct {
	loader  *confmaker.Loader
	handles map[*handleKey]any
}

// handleKey identifies one Provide call. It is not zero-sized: pointers to
// distinct zero-sized values may compare equal.
type handleKey struct{ _ byte }

// Module loads the configs registered by [Provide] and [ProvideNamed] through
// one confmaker.Loader per application, configured by opts. Loading happens once,
// when the application starts or when a constructor first needs a config, and
// its error lists the problems of every config, whatever order the options are
// given in.
//
// An application that calls Provide or ProvideNamed must include Module exactly
// once.
func Module(opts ...confmaker.EnvOption) fx.Option {
	opts = slices.Clone(opts)

	return fx.Module(
		"confmaker",

		fx.Provide(
			fx.Annotate(
				func(registrars []registrar) *registry {
					r := &registry{
						loader:  confmaker.MakeLoader(opts...),
						handles: make(map[*handleKey]any, len(registrars)),
					}

					// Every registration is added before anything can load.
					for _, register := range registrars {
						register(r)
					}

					return r
				},
				fx.ParamTags(utilfx.GroupTag(registrarGroup)),
			),
		),
	)
}

// Provide registers a config of type T with [Module] and provides it without an
// Fx tag. The instance name comes from confmaker.WithName or T's ConfigName;
// WithName adds no Fx tag. The config is loaded at start whether or not anything
// consumes it.
func Provide[T any](opts ...confmaker.ConfigOption) fx.Option {
	return provide[T](slices.Clone(opts), "")
}

// ProvideNamed registers a config of type T with [Module] and provides it with the
// Fx tag name:"<name>". name is also the instance name, and gives the prefix
// unless confmaker.WithPrefix changes it. ConfigName is not used, and a further
// WithName is reported as an error.
func ProvideNamed[T any](name string, opts ...confmaker.ConfigOption) fx.Option {
	return provide[T](slices.Concat([]confmaker.ConfigOption{confmaker.WithName(name)}, opts), utilfx.NameTag(name))
}

// provide adds three things to the application: the registration Module
// collects, the constructor that hands out the loaded value, and an Invoke that
// asks for it even when nothing else does - so an unused config is still loaded
// and a missing Module fails the start rather than going unnoticed.
func provide[T any](opts []confmaker.ConfigOption, tag string) fx.Option {
	key := new(handleKey)

	return fx.Options(
		utilfx.Grouped(registrarGroup, func() registrar {
			return func(r *registry) { r.handles[key] = r.loader.Add[T](opts...) }
		}),
		fx.Provide(fx.Annotate(
			func(r *registry) (T, error) { return loaded[T](r, key) },
			fx.ParamTags(optionalRegistry),
			fx.ResultTags(tag),
		)),
		fx.Invoke(fx.Annotate(
			func(r *registry) error {
				_, err := loaded[T](r, key)
				return err
			},
			fx.ParamTags(optionalRegistry),
		)),
	)
}

// loaded loads the application's configs on the first call and returns T's.
func loaded[T any](r *registry, key *handleKey) (T, error) {
	var cfg T
	if r == nil {
		return cfg, fmt.Errorf("config %s requires confx.Module() in the application", reflect.TypeFor[T]())
	}

	if err := r.loader.Load(); err != nil {
		return cfg, err
	}

	return r.handles[key].(*confmaker.Handle[T]).Value()
}

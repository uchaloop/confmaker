/*
Package confmaker builds typed configs from the environment. A library declares
its config as a struct with env tags; an application loads it, and the library
receives a ready value without reading the environment itself.

	cfg, err := confmaker.Load[store.Config]()

[Load] reads one config. A [Loader] reads several together and reports the
problems of all of them in one error. The Fx adapter is the separate module
github.com/uchaloop/confmaker/confx.

# Overview

A config is a struct with env tags and, optionally, three methods:

	type Config struct {
		Host     string        `env:"HOST,notEmpty"`
		Timeout  time.Duration `env:"TIMEOUT"`
		Password secret.Secret `env:"PASSWORD"`
	}

	func (Config) ConfigName() string { return "store" }
	func (c *Config) SetDefaults()    { c.Timeout = 30 * time.Second }
	func (c Config) Validate() error  { ... }

It reads STORE_HOST, STORE_TIMEOUT and STORE_PASSWORD. The tags are plain
strings, so the type depends on github.com/uchaloop/secret/v2 for the secret
field and not on this package.

# Declaring configurations

A field that names a variable in its env tag is read from that variable. A
struct field without an env tag is not read itself: its fields are, and those
with env tags become variables of the config, under the prefix extended by the
field's envPrefix tag, if any. Any other field without an env tag is not
configuration. env:"-" excludes a field and everything nested in it.

	type Config struct {
		Host string     `env:"HOST"`
		Pool PoolConfig `envPrefix:"POOL_"`
	}

	STORE_HOST
	STORE_POOL_MAX_CONNS

A nested config is nested by value: the variables a config reads are known from
its type, which a pointer or a collection could not promise. envPrefix is written
into names as it stands, with or without a trailing underscore.

A field may be a string, a bool, any sized integer or float, a time.Duration, a
type implementing encoding.TextUnmarshaler, a pointer to one of those, or a slice
or map of them. A TextUnmarshaler decodes its own text and reports its own
errors; for a manifest or dump it also needs encoding.TextMarshaler. complex,
uintptr and []byte are refused. A slice splits on envSeparator ("," by default).
A map splits entries the same way and each key from its value on
envKeyValSeparator (":" by default). The separator tags apply only to a slice or
map read in this plain syntax, not to a scalar, a type with its own text form or
a JSON field:

	STORE_BROKERS=a:9092,b:9092
	STORE_LABELS=env:prod,team:core

A duplicate map key is an error, and a key or value padded with whitespace is
reported rather than trimmed.

A set variable replaces its field. An empty variable is read like any other
text: a string becomes "", a slice or map becomes empty, a TextUnmarshaler
receives empty text, a number, bool or duration fails to parse, and a pointer
reads its element the same way.

Each field's variable is read under these tag options:

  - required: the variable must be set; an empty value is allowed;
  - notEmpty: the variable must be set and its text must be non-empty. It checks
    the text, not the result: JSON [] is non-empty text for an empty slice. Limits
    on what a value holds belong in Validate.

A default does not satisfy either option: they concern what the deployment
supplies.

Declarations are checked when a config is registered, before any default or
variable is read, and a mistake is reported by [Loader.Load] or [Manifest]:
envDefault (defaults belong in SetDefaults), an unknown option or options
without a name, two fields on one variable, envPrefix on anything but a struct
nested by value, a secret in a slice, array or map, a config nested through a
pointer or collection, an unreadable type, an empty separator or one on a field
that is not split, and a variable name no environment can hold (with = or NUL).
Secrets are recognised behind any number of pointers.

# Names and prefixes

Every config has an instance name. It gives the default prefix - "read-replica"
reads READ_REPLICA_* - and labels the config in errors. It comes from
[WithName] or, without it, from the config's ConfigName method ([ConfigNamer]):

	loader.Add[Config]()                                 // "store", STORE_*
	loader.Add[Config](confmaker.WithName("replica"))    // "replica", REPLICA_*
	loader.Add[Config](confmaker.WithPrefix("MAIN_DB_")) // "store", MAIN_DB_*

Names hold lowercase letters, digits and _ - ., and do not start or end with a
separator. A config with neither WithName nor ConfigName cannot be registered.
Two registrations may not share a name, a prefix, or a variable.

Options come in two kinds: a [ConfigOption] configures one config, an
[EnvOption] configures a whole load. Passing one where the other is expected does
not compile. [Load] takes both.

# Defaults and validation

Loading a config runs these steps:

 1. SetDefaults, if the config has it, sets defaults in code.
 2. Each variable that is set replaces its field; an unset variable leaves the
    default.
 3. Validate, if the config has it, checks the result. It runs only when every
    variable of the config parsed.

SetDefaults must be deterministic and free of side effects: loading calls it
once per config, and every [Manifest] call calls it again. Only the top-level
config's methods are called.

# JSON values

envFormat:"json" reads one variable as JSON, with encoding/json/v2, into a
struct, slice, array or map, or a pointer to one, nested to any depth:

	type Config struct {
		Endpoints []Endpoint `env:"ENDPOINTS" envFormat:"json"`
	}

	STORE_ENDPOINTS=[{"url":"http://a:9000","timeout":"30s"}]

The value is decoded into the field's type:

  - A set variable replaces the whole field; nothing is merged with the default.
    {"url":"http://a"} leaves timeout zero even when the default had one.
  - Member names match exactly, and unknown or duplicate members are errors,
    unless a field's JSON tag says otherwise: case:ignore matches a name in any
    case, and an embed map collects members no field names, whatever they are.
  - Only json:"-" ignores a field; json:"-," and json:"-,omitempty" name it "-".
  - null clears a pointer, slice or map and is an error elsewhere. [] and {} are
    empty collections. An empty variable is an error.
  - A time.Duration is a string such as "30s", in collections too.
  - A type with UnmarshalJSON or UnmarshalText reads its own form, and is as
    strict as it decides to be.

Refused at registration: envFormat on a scalar, an unknown format, envFormat with
a separator tag, interface values, []byte and byte arrays, map keys other than
strings, integers or text types, any secret type inside the value, ignored
fields included (such a secret is refused, not masked), and a struct json/v2
rejects as an object - two fields on one JSON name, for example. The last check
decodes {} into each struct type, which runs no method of the config.

An error names the variable and the place inside the value:

	config "store": variable "STORE_ENDPOINTS": JSON at "/0/timeout": time: invalid duration "soon"

A manifest renders a JSON default with sorted map keys and nil as null. Whether
it loads back to an equal value depends on the type: custom marshalers decide
their own form, and omitempty drops an empty slice or map, which then loads back
as nil. omitzero keeps an empty non-nil collection.

# Loading and lifecycle

[Loader.Load] runs in this order:

 1. Invalid options and registrations are reported; the other configs still
    load.
 2. Conflicting registrations - two on one name, prefix or variable - end loading
    here, before any SetDefaults runs or anything is printed.
 3. One snapshot of the environment is taken.
 4. SetDefaults runs for each config.
 5. The dump is written, when [WithDump] asks for one. A dump that fails is
    reported and does not stop the checks.
 6. Variables under a registered prefix that no field reads are reported.
 7. The environment is applied and Validate runs, for each config.

Everything found is joined into one error, one problem per line, ordered by
instance name and prefix rather than by the order of Add calls:

	unknown configuration variable "STORE_HSOT" (did you mean "STORE_HOST"?)
	config "store": required variable "STORE_HOST" is not set

Values are handed out only when the whole set loaded; see [Handle.Value]. Load
runs once. A [Loader] is safe for concurrent use and holds no lock while
ConfigName, SetDefaults, Validate, unmarshalers or the dump writer run; those may
call Add or Value on the same Loader but must not call its Load.

# Environment and testing

Load reads one snapshot of the environment, shared by loading, the check for
unknown variables and the dump. By default it is the process environment;
[WithEnv] replaces it with a map, which suits tests:

	cfg, err := confmaker.Load[Config](confmaker.WithEnv(map[string]string{
		"STORE_HOST": "localhost",
	}))

confmaker reads variables only; reading a file into such a map is left to the
caller. [AllowUnknown] exempts prefixes from the check for unknown variables, for
an environment shared with other programs.

# Manifest, dump and secrets

[Manifest] and [WithDump] describe the same variables for different purposes:

  - Manifest returns metadata for generating a .env.example, a config map or a
    table. It does not read the environment, but it calls SetDefaults and renders
    each default with MarshalText or the field's plain syntax.
  - WithDump prints, while loading, each variable's environment value or
    default and its source. It is an input report, not proof that loading
    succeeded. Control characters are escaped.

A default that the field's separators could not carry back is an error.
[Variable.HasDefault] reports a non-zero default and cannot tell an explicit
false, 0 or "" from no default. Manifest values are ENV text; a generator escapes
them for shell or YAML itself.

A field of a secret type (github.com/uchaloop/secret/v2) is never printed: not in
the dump, the manifest or a parse error. Secrets are recognised by type only. A
password written into a plain string, such as postgres://user:password@host/db,
is printed like any other text; keep it in its own secret field.
*/
package confmaker

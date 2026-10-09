/*
Package confmaker centralizes explicit configuration of an application's packages,
modules and domains. Each package declares a struct with env tags; the application
registers its configs with instance names, loads them together and passes typed
values to their consumers. Packages do not need to read ENV themselves.

Import the core library:

	import "github.com/uchaloop/confmaker/v2"

A [Loader] is the composition point for multiple configurations, including
multiple instances of the same type:

	loader := confmaker.MakeLoader()
	primary := loader.Register[store.Config]("postgres")
	replica := loader.Register[store.Config]("replica")
	if err := loader.Load(); err != nil {
		return err
	}
	primaryConfig, err := primary.Value()
	if err != nil {
		return err
	}
	replicaConfig, err := replica.Value()
	if err != nil {
		return err
	}
	// Pass primaryConfig and replicaConfig to their consumers.

[Load] is a convenience for loading one configuration. The separate module
github.com/uchaloop/confx adapts the same loading rules to Uber Fx. This package
reads ENV only: it does not read dotenv files, contact secret stores or start
application services. Manifest generation and exports are optional.

# Overview

A config is a struct with env tags and, optionally, two methods:

	type Config struct {
		Host     string        `env:"HOST,notEmpty"`
		Timeout  time.Duration `env:"TIMEOUT"`
		Password secret.Secret `env:"PASSWORD"`
	}

	func (c *Config) SetDefaults() { c.Timeout = 30 * time.Second }
	func (c Config) Validate() error {
		if c.Timeout <= 0 {
			return fmt.Errorf("timeout must be positive")
		}
		return nil
	}

With instance name "store", it reads STORE_HOST, STORE_TIMEOUT and
STORE_PASSWORD. The tags are plain strings, so the config type need not import confmaker.
This example imports github.com/uchaloop/secret/v2 for its secret field.

# Declaring configurations

A field that names a variable in its env tag is read from that variable. A
struct field without an env tag is not read itself: its fields are, and those
with env tags become variables of the config, under the prefix extended by the
field's envPrefix tag, if any. Untagged fields that contain no configuration
are ignored; unsupported pointer or collection nesting of config fields is
rejected. env:"-" excludes a field and everything nested in it.

	type Config struct {
		Host string     `env:"HOST"`
		Pool PoolConfig `envPrefix:"POOL_"`
	}

	STORE_HOST
	STORE_POOL_MAX_CONNS

A nested config must be held by value. This gives the application a fixed set
of fields independent of pointer allocation or collection contents. envPrefix is written
into names as it stands, with or without a trailing underscore.

A field may be a string, a bool, any sized integer or float, a time.Duration, a
type implementing encoding.TextUnmarshaler, a pointer to one of those, or a slice
or map of them. A TextUnmarshaler decodes its own text and reports its own
errors; for a manifest it also needs encoding.TextMarshaler. complex,
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

  - required: the variable must be set; empty text is allowed if its type can parse it;
  - notEmpty: the variable must be set and its text must be non-empty. It checks
    the text, not the result: JSON [] is non-empty text for an empty slice. Limits
    on what a value holds belong in Validate.

A default does not satisfy either option: they concern what the deployment
supplies.

Pointer-only type cycles are rejected as declaration errors. Pointers to scalar
values remain supported, including multiple levels. Untagged nested configs must
be structs held by value; hiding them behind pointers or collections at any
depth is rejected. Plain ENV maps cannot use pointer keys, whose address
identity would bypass duplicate-key checks. Parsed keys must equal themselves:
NaN keys, including custom keys containing NaN, are rejected. NaN values remain
valid. Ordinary structs inside JSON are rejected.

Declarations are checked when a config is registered, before any default or
variable is read, and a mistake is reported by [Loader.Load], [Loader.Manifest] or [Manifest]:
envDefault (defaults belong in SetDefaults), an unknown option or options
without a name, two fields on one variable, envPrefix on anything but a struct
nested by value, a secret in a slice, array or map, a config nested through a
pointer or collection, an unreadable type, an empty separator or one on a field
that is not split, and a variable name no environment can hold (with = or NUL).
Secrets are recognised behind any number of pointers.

# Names and prefixes

Every config has an instance name. It gives the default prefix - "read-replica"
reads READ_REPLICA_* - and labels the config in errors. It comes from
the required first argument of [Loader.Register], [Load] or [Manifest]:

	loader.Register[Config]("store")   // STORE_*
	loader.Register[Config]("replica") // REPLICA_*
	loader.Register[Config]("main", confmaker.WithPrefix("MAIN_DB_")) // MAIN_DB_*

Names hold lowercase letters, digits and _ - ., and do not start or end with a
separator. An empty name is invalid, even with WithPrefix.
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
once per config, and each manifest or export call evaluates it again on a fresh
instance. Only the top-level
config's methods are called.

For use outside the loader, a package can expose a defaults factory and delegate
SetDefaults to it:

	func DefaultConfig() Config {
		return Config{Timeout: 30 * time.Second}
	}

	func (c *Config) SetDefaults() {
		*c = DefaultConfig()
	}

Tests and manually assembled applications can call DefaultConfig, apply their
own overrides and call Validate explicitly. confmaker discovers only SetDefaults,
not the factory. Config{} does not apply defaults. This SetDefaults implementation
replaces the entire value, so it must run before overrides. Keep invariants that
must hold outside ENV loading in Validate: required and notEmpty check the supplied
ENV, not a manually constructed config.

# JSON collections

envFormat:"json" reads a slice, array or map from one variable. A pointer to
one of these collections is also supported. Nested collections are allowed:

	type Config struct {
	 Headers map[string]string `env:"HEADERS" envFormat:"json"`
	 Groups map[string][]string `env:"GROUPS" envFormat:"json"`
	}

	APP_HEADERS={"X-Service":"catalog"}
	APP_GROUPS={"read":["primary","replica"]}

A supplied value replaces the entire default. Duplicate keys and empty input
are errors. null clears pointers, maps and slices; it is invalid for other
values. Durations use strings such as "30s". Map keys must be strings, integers
or supported text types. Defaults render with deterministic map ordering.

Ordinary structs are rejected at every depth, including collection elements.
Declare nested configuration by value with envPrefix instead. Types implementing
encoding.TextUnmarshaler may represent scalar text values, including time.Time;
they are read from JSON strings using UnmarshalText. Rendering their defaults
requires encoding.TextMarshaler; otherwise manifest returns ErrorDefaultRender.
Text methods take precedence over JSON methods in both directions. Custom text
methods must agree on a representation that can be read back.
A JSON-only custom decoder does not exempt a struct from this restriction.

Interfaces, byte collections without a custom text form, unsupported kinds and
secret types are rejected. Secrets hidden inside custom text types are rejected
too. envFormat cannot be combined with separator tags. Errors identify the ENV
variable and location in the JSON value, preserving the underlying JSON error.

# Loading and lifecycle

[Loader.Load] runs in this order:

 1. Invalid options and registrations are reported; the other configs still
    load.
 2. Conflicting registrations - two on one name, prefix or variable - end loading
    here, before any SetDefaults runs.
 3. One snapshot of the environment is taken.
 4. SetDefaults runs for each config.
 5. Variables under a registered prefix that no field reads are reported.
 6. The environment is applied and Validate runs, for each config.

Problems are joined into one error. Config registrations are processed in a
deterministic order rather than the order of Register calls; unknown variables
are sorted by name. Errors from different stages retain their stage order:

	unknown configuration variable "STORE_HSOT" (did you mean "STORE_HOST"?)
	config "store": required variable "STORE_HOST" is not set

Values are handed out only when the whole set loaded; see [Handle.Value]. Load
runs once. A [Loader] is safe for concurrent use and holds no lock while
SetDefaults, Validate or unmarshalers run; those may
call Register on that Loader or Value on its handles, but must not call its Load.

# Environment and testing

Load reads one snapshot of the environment, shared by field parsing and the
check for unknown variables. By default it is the process environment;
[WithEnv] replaces it with a map, which suits tests:

	cfg, err := confmaker.Load[Config]("store", confmaker.WithEnv(map[string]string{
		"STORE_HOST": "localhost",
	}))

confmaker reads variables only; reading a file into such a map is left to the
caller. Passing nil to WithEnv means an empty environment, not a fallback to
the process. WithEnv copies the map when the option is created.
[AllowUnknown] exempts prefixes from the check for unknown variables, for
an environment shared with other programs.

# Structured errors

[ConfigError] exposes an [ErrorKind], InstanceName, VariableName and FieldPath.
Context is empty when unavailable. errors.As finds the first structured problem;
[ConfigErrors] walks wrappers and joins to collect all problems in tree order.
Each ConfigError counts as one problem, including a Validate error whose cause
is itself a join. Its original cause remains reachable through errors.Is and
errors.As. Returned pointers should be treated as read-only.

Standard scalar parsers retain their original causes without changing the
human-readable message. Bool, integer and float errors expose strconv.NumError
and its syntax or range cause. Duration errors retain the time parser's cause.

Secret parse causes are discarded rather than exposed through Unwrap. Context
fields do not contain ENV values, but ordinary error messages and user Validate
errors may contain them. Writer and lifecycle errors remain separate; always
handle the original error even when ConfigErrors returns no problems.

# Load reports

[WithDiagnostics] collects value-free [LoadReport] snapshots in a
[Diagnostics] receiver. [WithDiagnosticHandler] handles a completed report
synchronously before Load returns, on success or error. Neither option reads
ENV again or repeats defaults, parsing or validation. Reports contain sources,
statuses and error categories, never values, messages or causes. Collection is
optional and independent of manifest generation. A receiver belongs to one
loader; Report is safe for concurrent use and returns independent copies.
Loading panics remain panics and skip the handler; the receiver records
[LoadPanicked]. See [WithDiagnosticHandler] for completion and callback ordering.

# Manifest and secrets

[Loader.Manifest] describes a snapshot of all registrations as []ConfigManifest,
ordered by instance name, with variables in declaration order. It checks invalid
registrations and conflicts before invoking config methods. Any declaration,
conflict or rendering error returns a nil result. Load options, including WithEnv, are ignored.

	configs, err := loader.Manifest()

It can run before, during or after Load, without reading ENV, calling Validate
or changing loader state. Each call evaluates defaults on fresh
instances and never reads loaded values. Registrations completed after the
snapshot are included on the next call. If called concurrently with Load or
another Manifest, user-supplied defaults and marshalers must support concurrent
calls. No loader lock is held while those methods run.

[Loader.WriteEnvExample] writes a reference .env.example to an io.Writer using
that same manifest. The envDescription tag supplies [Variable.Description] and
comments in the template. Safe non-zero scalar defaults become assignments;
secrets, complex values and defaults requiring quoting become commented
placeholders. Zero defaults are indistinguishable from absent defaults. Names
unsuitable for dotenv assignments are preserved in quoted comments. This does
not impose additional restrictions on names accepted by Load. The caller owns
file handling; no dotenv file is read automatically. Manifest errors write
nothing; writer errors may leave partial output.

[MakeDescribeFlag] optionally registers -describe on a caller-supplied
flag.FlagSet. After parsing, check DescribeFlag.Requested and call
DescribeFlag.Write to export env, markdown or json. Unknown formats fail flag
parsing. The helper does not parse arguments, load configs or exit the process.

[Loader.WriteManifestJSON] exports a versioned JSON document with fixed
camelCase keys, preserving all variable metadata except secret defaults.
[Loader.WriteManifestMarkdown] exports sections and tables for documentation,
escaping text and distinguishing required from required non-empty variables.
Both use the same Manifest lifecycle and ordering, finish preparation before
writing and leave file handling to the caller. Writer failures can leave partial
output. JSON's hasDefault retains the non-zero semantics described below.

[Manifest] describes declarations and defaults without reading ENV.
[Diagnostics] describes a load's sources and results without recording values.

Manifest and all exporters are strict: if any default cannot be rendered,
the whole description fails. This includes nil pointers anywhere in a plain
collection element or a default that its separators cannot represent. A
commented .env.example placeholder does not bypass that check. Ordinary loading
does not require defaults to be rendered.
[Variable.HasDefault] reports a non-zero default and cannot tell an explicit
false, 0 or "" from no default. Manifest values are ENV text; a generator escapes
them for shell or YAML itself.

Secret defaults (github.com/uchaloop/secret/v2) are excluded from manifest
exports and secret parse values are omitted from library errors. Ordinary defaults
are included in manifest exports; user validation errors may contain values.
Keep credentials out of non-secret defaults and user-generated error messages.
*/
package confmaker

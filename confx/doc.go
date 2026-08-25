/*
Package confx builds a library's typed config from the environment and provides
it into an Uber Fx container, so the library consumes a ready config without
reading the environment itself.

# Declaring a config

A config is a plain struct with env tags. The tags are inert strings, so the
type depends only on the secret type (github.com/uchaloop/secret/v2), not on
this package:

	type Config struct {
		Host     string        `env:"HOST,notEmpty"`
		Timeout  time.Duration `env:"TIMEOUT"`
		Password secret.Secret `env:"PASSWORD"`
	}

A field without an env tag is not configuration and is never touched. `env:"-"`
takes a field out of the config together with anything nested in it.

Field types are string, bool, every sized integer and float, time.Duration,
anything implementing encoding.TextUnmarshaler, a pointer to any of those, and
a slice or map of them. TextUnmarshaler is the extension point: a decimal, a
nullable, a UUID decodes itself, reports its own reason for refusing a value,
and renders itself back to text, with no parser to register. complex, uintptr
and []byte are refused rather than guessed at.

# Building one

Provide runs three optional steps. The config establishes its own defaults
through SetDefaults, the environment overrides what it set, and Validate checks
the result:

	func (c *Config) SetDefaults() { c.Timeout = 30 * time.Second }

	func (c Config) Validate() error { ... }

Three rules govern the middle step:

  - a variable that is not set leaves its field as SetDefaults left it;
  - a variable that is set assigns its field;
  - a variable set to the empty string assigns the empty value.

That is why defaults belong in SetDefaults rather than in a tag: a default in a
tag is visible only to the loader, while a method is visible to the library's
own tests and callers too. There is no envDefault tag, and declaring one is an
error.

# Required values

The option require fails the start when the variable is not set; notEmpty fails
when it is not set or is empty. Both are about the variable rather than the
value, so a default in SetDefaults satisfies neither - the deployment is still
expected to supply it. For a string, notEmpty is almost always the one you want:
an empty value in a config map is the commonest way to set a variable and set
nothing.

A Validate method may also refuse an empty value, and the overlap is deliberate.
The tag speaks to a deployment - it names the variable and fires before anything
is built - while Validate speaks to any caller, including one that builds the
config in Go and never goes near a loader.

# Instances and prefixes

Provide gives the container an untagged value, the single default instance.
ProvideNamed gives it a value tagged name:"<name>", for replicas and additional
instances of the same type. The instance name gives the environment prefix, so
Provide[Config]("postgres") reads POSTGRES_HOST and ProvideNamed[Config]("replica")
reads REPLICA_HOST.

Because the name is both the prefix and the Fx tag, it may hold only lowercase
letters, digits and _ - ., and may not start or end with a separator. Whichever
separator it uses, the variable is written with underscores, so read-replica and
read_replica both arrive at READ_REPLICA_ - which is why two instances may not be
named that way at once. WithPrefix overrides the prefix when it should not follow
the name.

# Nesting, slices and maps

envPrefix extends the prefix for a nested struct:

	type Config struct {
		Host string     `env:"HOST"`
		Pool PoolConfig `envPrefix:"POOL_"`
	}

	POSTGRES_HOST
	POSTGRES_POOL_MAX_CONNS

Nest by value: how many variables a config reads has to be known from its type,
which a pointer or a collection cannot promise.

A slice splits on envSeparator, "," by default. A map reads from one variable,
splitting entries the same way and a key from its value on envKeyValSeparator,
":" by default:

	OZON_BROKERS=a:9092,b:9092
	OZON_LABELS=env:prod,team:core

A duplicate key is an error, and a key or value padded with whitespace is
reported rather than trimmed, so "env: prod" says so instead of quietly yielding
" prod".

# What is refused

What a declaration can get wrong is refused when the config is bound, on the
first start, whether or not the environment happens to set the variable:

  - envDefault on a field: a default belongs in SetDefaults;
  - an option other than require or notEmpty, or options with no variable name:
    a misspelled one would leave the field quietly optional;
  - two fields claiming one variable: one value would fill both;
  - a config nested through a pointer, slice, map or array;
  - a field of a type that cannot be read from text;
  - an empty envSeparator: a value would split into single characters;
  - an instance name or prefix that is not one.

Every problem is reported at once - across binding, parsing and validation
alike - one per line, each naming the config it belongs to, so a misconfigured
deployment takes one rollout to diagnose rather than one per mistake.

# Checking the environment

Module compares the environment against the manifest the Provide calls register.
A variable that starts with a prefix the application owns but matches no field
fails the start:

	unknown configuration variable "POSTGRES_HSOT" (did you mean "POSTGRES_HOST"?)

Variables outside the application's own prefixes are never examined. Register
Module before the Provide calls it covers, so the check runs ahead of the
constructors and reports the typo rather than the missing value it causes.
AllowUnknown exempts a prefix, for a deployment that shares one environment
between several binaries.

WithDump writes every variable the application reads, its type, its current
value and where that value came from. A secret is never printed: not in the
dump, not in the manifest, not in the error for a value that would not parse,
and it publishes no default.

# Generating from the manifest

Manifest resolves the same list without building an application, so a
.env.example, a config map or a documentation table comes from the config type
itself. It is the traversal that fills the config, so a variable it lists is
exactly a variable the config reads.
*/
package confx

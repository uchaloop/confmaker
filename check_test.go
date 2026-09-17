package confmaker

import (
	"bytes"
	"math/rand"
	"reflect"
	"strings"
	"testing"

	"github.com/uchaloop/secret/v2"
)

// strictConfig is the config the strict-check tests register.
type strictConfig struct {
	Host string        `env:"HOST"`
	Port int           `env:"PORT"`
	Pass secret.Secret `env:"PASSWORD,required"`
}

// SetDefaults gives Port a default the dump can report as coming from the code.
func (c *strictConfig) SetDefaults() {
	c.Port = 5432
}

// Tests pass their environment through WithEnv, so they read nothing the machine
// running them happens to export and can run in parallel. The confx-prefixed
// names date from when they read the process environment.

// loadStrict loads the strictConfig instance "confxpostgres" from env.
func loadStrict(env map[string]string, opts ...EnvOption) error {
	loadOpts := []LoadOption{WithName("confxpostgres"), WithEnv(env)}
	for _, opt := range opts {
		loadOpts = append(loadOpts, opt)
	}

	_, err := Load[strictConfig](loadOpts...)

	return err
}

func TestCheckAcceptsDeclaredVariables(t *testing.T) {
	t.Parallel()

	env := map[string]string{
		"CONFXPOSTGRES_HOST":     "db:5432",
		"CONFXPOSTGRES_PASSWORD": "s3cr3t",
	}

	if err := loadStrict(env); err != nil {
		t.Fatalf("declared variables rejected: %v", err)
	}
}

func TestCheckRejectsTypoUnderOwnPrefix(t *testing.T) {
	t.Parallel()

	env := map[string]string{
		"CONFXPOSTGRES_HOST":     "db:5432",
		"CONFXPOSTGRES_PASSWORD": "s3cr3t",
		"CONFXPOSTGRES_HSOT":     "typo",
	}

	err := loadStrict(env)
	if err == nil {
		t.Fatal("expected the typo to fail the load")
	}

	if !strings.Contains(err.Error(), "CONFXPOSTGRES_HSOT") {
		t.Fatalf("error does not name the variable: %v", err)
	}

	if !strings.Contains(err.Error(), `did you mean "CONFXPOSTGRES_HOST"`) {
		t.Fatalf("error does not suggest the intended variable: %v", err)
	}
}

func TestCheckIgnoresForeignPrefixes(t *testing.T) {
	t.Parallel()

	env := map[string]string{
		"CONFXPOSTGRES_HOST":     "db:5432",
		"CONFXPOSTGRES_PASSWORD": "s3cr3t",
		"KAFKA_BROKERS":          "not ours",
	}

	if err := loadStrict(env); err != nil {
		t.Fatalf("a variable outside the application's prefixes was reported: %v", err)
	}
}

func TestAllowUnknownExemptsPrefix(t *testing.T) {
	t.Parallel()

	env := map[string]string{
		"CONFXPOSTGRES_HOST":         "db:5432",
		"CONFXPOSTGRES_PASSWORD":     "s3cr3t",
		"CONFXPOSTGRES_EXPORTER_URL": "sidecar",
	}

	if err := loadStrict(env, AllowUnknown("CONFXPOSTGRES_EXPORTER_")); err != nil {
		t.Fatalf("exempted prefix still reported: %v", err)
	}
}

func TestCheckCoversEveryInstance(t *testing.T) {
	t.Parallel()

	env := map[string]string{
		"CONFXMAIN_HOST":        "db:5432",
		"CONFXMAIN_PASSWORD":    "s3cr3t",
		"CONFXREPLICA_HOST":     "replica:5432",
		"CONFXREPLICA_PASSWORD": "s3cr3t",
		"CONFXREPLICA_PROT":     "typo",
	}

	loader := MakeLoader(WithEnv(env))
	loader.Add[strictConfig](WithName("confxmain"))
	loader.Add[strictConfig](WithName("confxreplica"))
	err := loader.Load()
	if err == nil || !strings.Contains(err.Error(), "CONFXREPLICA_PROT") {
		t.Fatalf("a named instance was not covered by the check: %v", err)
	}
}

func TestInstanceWithoutPrefixIsRefused(t *testing.T) {
	t.Parallel()

	// Every instance owns a prefix, so the scan has no exception to make: an
	// instance with no prefix would claim the whole environment and is refused
	// where it is declared.
	loader := MakeLoader(WithEnv(nil))
	loader.Add[strictConfig](WithName("confxpostgres"), WithPrefix(""))
	err := loader.Load()
	if err == nil {
		t.Fatal("an instance with no prefix was accepted")
	}
}

func TestNestedPrefixesDoNotCollide(t *testing.T) {
	t.Parallel()

	env := map[string]string{
		"CONFXAPI_BASE_URL":        "https://api",
		"CONFXAPI_STATUS_BASE_URL": "https://api/cards",
	}

	type outer struct {
		BaseURL string `env:"BASE_URL"`
	}

	// CONFXAPI_ and CONFXAPI_STATUS_ overlap: a variable of the longer instance
	// must not be reported as unknown for the shorter one.

	loader := MakeLoader(WithEnv(env))
	loader.Add[outer](WithName("confxapi"))
	loader.Add[outer](WithName("confxapi_status"))
	err := loader.Load()
	if err != nil {
		t.Fatalf("overlapping prefixes reported a false positive: %v", err)
	}
}

func TestWithDumpListsVariablesAndMasksSecrets(t *testing.T) {
	t.Parallel()

	env := map[string]string{
		"CONFXPOSTGRES_HOST":     "db:5432",
		"CONFXPOSTGRES_PASSWORD": "s3cr3t",
	}

	var out bytes.Buffer

	_, err := Load[strictConfig](WithDump(&out), WithEnv(env), WithName("confxpostgres"))
	if err != nil {
		t.Fatalf("app: %v", err)
	}

	dump := out.String()
	for _, want := range []string{
		"CONFXPOSTGRES_HOST", "db:5432",
		"CONFXPOSTGRES_PORT", "5432", "default",
		"CONFXPOSTGRES_PASSWORD", "(set)",
	} {
		if !strings.Contains(dump, want) {
			t.Errorf("dump is missing %q:\n%s", want, dump)
		}
	}

	if strings.Contains(dump, "s3cr3t") {
		t.Fatalf("dump printed a secret value:\n%s", dump)
	}
}

func TestHintIsDeterministic(t *testing.T) {
	// Three candidates sit at the same edit distance from the typo, so an
	// unordered scan of the known names would report a different one per run.
	known := map[string]string{"CONFXAPP_HOST": "app", "CONFXAPP_MOST": "app", "CONFXAPP_COST": "app"}

	first := hint("CONFXAPP_XOST", known)
	for range 100 {
		if got := hint("CONFXAPP_XOST", known); got != first {
			t.Fatalf("hint changed between runs: %q then %q", first, got)
		}
	}

	if !strings.Contains(first, "CONFXAPP_COST") {
		t.Fatalf("hint = %q, want the first candidate by name among the ties", first)
	}
}

func TestAllowUnknownIgnoresEmptyPrefix(t *testing.T) {
	t.Parallel()

	env := map[string]string{
		"CONFXPOSTGRES_HOST":     "db:5432",
		"CONFXPOSTGRES_PASSWORD": "s3cr3t",
		"CONFXPOSTGRES_HSOT":     "typo",
	}

	// An empty prefix matches everything; honouring it would turn the check off
	// without saying so.
	err := loadStrict(env, AllowUnknown(""))
	if err == nil || !strings.Contains(err.Error(), "CONFXPOSTGRES_HSOT") {
		t.Fatalf("an empty prefix silently disabled the check: %v", err)
	}
}

func TestNonStructConfigIsNotBlamedOnTheEnvironment(t *testing.T) {
	t.Parallel()

	env := map[string]string{
		"CONFXTHING_HOST": "db:5432",
	}

	// A pointer T is a mistake in the code, not in the environment. The check
	// must not bury the parser's own report under a list of "unknown" variables.
	loader := MakeLoader(WithEnv(env))
	loader.Add[*strictConfig](WithName("confxthing"))
	err := loader.Load()
	if err == nil {
		t.Fatal("expected a pointer config to fail")
	}

	if strings.Contains(err.Error(), "unknown configuration variable") {
		t.Fatalf("the environment was blamed for a mistake in the code: %v", err)
	}
}

func TestUnreadableConfigIsReported(t *testing.T) {
	t.Parallel()

	type shard struct {
		Host string `env:"HOST"`
	}

	type config struct {
		Name   string `env:"NAME"`
		Shards []shard
	}

	// A collection of structs would read variables no type can enumerate, so the
	// declaration is refused instead of quietly leaving a hole in the check.
	loader := MakeLoader(WithEnv(nil))
	loader.Add[config](WithName("confxcluster"))
	err := loader.Load()
	if err == nil || !strings.Contains(err.Error(), "nest by value") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestTwoInstancesMayNotClaimOneVariable covers the collision a single config
// cannot see: two prefixes that run into each other, so a name declared under
// one instance is the name another instance reads. One value would fill two
// configs that nothing keeps in step.
func TestTwoInstancesMayNotClaimOneVariable(t *testing.T) {
	type outer struct {
		Host string `env:"MAIN_HOST"`
	}

	type inner struct {
		Host string `env:"HOST"`
	}

	_, err := checkRegistrations([]descriptor{
		{label: "db", prefix: "CONFXDB_", fields: mustDescribe[outer](t, "CONFXDB_")},
		{label: "db_main", prefix: "CONFXDB_MAIN_", fields: mustDescribe[inner](t, "CONFXDB_MAIN_")},
	})

	if err == nil {
		t.Fatal("the shared variable was accepted")
	}

	if !strings.Contains(err.Error(), "CONFXDB_MAIN_HOST") {
		t.Fatalf("the error does not name the variable: %v", err)
	}

	if !strings.Contains(err.Error(), `"db"`) || !strings.Contains(err.Error(), `"db_main"`) {
		t.Fatalf("the error does not name both instances: %v", err)
	}
}

func TestDistinctInstancesAreNotACollision(t *testing.T) {
	type config struct {
		Host string `env:"HOST"`
	}

	_, err := checkRegistrations([]descriptor{
		{label: "primary", prefix: "CONFXPRIMARY_", fields: mustDescribe[config](t, "CONFXPRIMARY_")},
		{label: "replica", prefix: "CONFXREPLICA_", fields: mustDescribe[config](t, "CONFXREPLICA_")},
	})
	if err != nil {
		t.Fatalf("two instances of one type were refused: %v", err)
	}
}

func mustDescribe[T any](t *testing.T, prefix string) []fieldSpec {
	t.Helper()

	fields, err := compileSchema(reflect.TypeFor[T](), prefix)
	if err != nil {
		t.Fatal(err)
	}

	return fields
}

// fullMatrixDistance is the textbook Levenshtein, kept as the reference the
// bounded scan is checked against. The scan fills only a band around the
// diagonal and abandons a comparison the moment it cannot come in under budget,
// so what it skips has to be shown to be what it was allowed to skip.
func fullMatrixDistance(a, b string) int {
	previous := make([]int, len(b)+1)
	current := make([]int, len(b)+1)

	for j := range previous {
		previous[j] = j
	}

	for i := 1; i <= len(a); i++ {
		current[0] = i

		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}

			current[j] = min(previous[j]+1, current[j-1]+1, previous[j-1]+cost)
		}

		previous, current = current, previous
	}

	return previous[len(b)]
}

func TestEditDistanceMatchesTheFullMatrix(t *testing.T) {
	// Four symbols over short words, so the pairs that matter - a transposition,
	// a repeated run, one string a prefix of the other - come up in bulk.
	const alphabet = "AB_0"

	random := rand.New(rand.NewSource(1))

	word := func() string {
		out := make([]byte, random.Intn(13))
		for i := range out {
			out[i] = alphabet[random.Intn(len(alphabet))]
		}

		return string(out)
	}

	var rows editRows

	for range 50000 {
		a, b := word(), word()

		for limit := range 5 {
			want := min(fullMatrixDistance(a, b), limit+1)

			if got := rows.editDistance(a, b, limit); got != want {
				t.Fatalf("editDistance(%q, %q, %d) = %d, want %d", a, b, limit, got, want)
			}
		}
	}
}

// TestEditDistanceIsSymmetric covers the swap the scan makes to put the longer
// string first: a distance that depended on the order would make a suggestion
// depend on which name the map happened to yield.
func TestEditDistanceIsSymmetric(t *testing.T) {
	const alphabet = "AB_0"

	random := rand.New(rand.NewSource(2))

	var rows editRows

	for range 20000 {
		a := make([]byte, random.Intn(16))
		b := make([]byte, random.Intn(16))

		for i := range a {
			a[i] = alphabet[random.Intn(len(alphabet))]
		}

		for i := range b {
			b[i] = alphabet[random.Intn(len(alphabet))]
		}

		forward := rows.editDistance(string(a), string(b), maxHintDistance)
		if backward := rows.editDistance(string(b), string(a), maxHintDistance); forward != backward {
			t.Fatalf("editDistance(%q, %q) = %d but reversed = %d", a, b, forward, backward)
		}
	}
}

// TestEditRowsAreReusable covers the buffers a scan carries from one candidate
// to the next: a row left over from a longer name must not be read as part of a
// shorter one.
func TestEditRowsAreReusable(t *testing.T) {
	var rows editRows

	for _, pair := range [][2]string{
		{"CONFXAPP_A_VERY_LONG_VARIABLE_NAME", "CONFXAPP_A_VERY_LONG_VARIABLE_NAMF"},
		{"CONFXAPP_HOST", "CONFXAPP_HSOT"},
		{"A", "B"},
		{"", ""},
		{"CONFXAPP_HOST", "CONFXAPP_HOST"},
	} {
		want := min(fullMatrixDistance(pair[0], pair[1]), maxHintDistance+1)

		if got := rows.editDistance(pair[0], pair[1], maxHintDistance); got != want {
			t.Fatalf("editDistance(%q, %q) = %d, want %d", pair[0], pair[1], got, want)
		}
	}
}

func TestHintStaysSilentWithoutACloseCandidate(t *testing.T) {
	known := map[string]string{"CONFXAPP_HOST": "app"}

	if got := hint("CONFXAPP_SOMETHING_ENTIRELY_ELSE", known); len(got) != 0 {
		t.Fatalf("a distant name was suggested: %q", got)
	}
}

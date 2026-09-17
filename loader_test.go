package confmaker

import (
	"bytes"
	"errors"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type loaderConfig struct {
	Host string `env:"HOST,notEmpty"`
}

var loaderDefaultCalls atomic.Int32

func (*loaderConfig) SetDefaults() { loaderDefaultCalls.Add(1) }

type loaderOtherConfig struct {
	Port int `env:"PORT,required"`
}

func TestValueBeforeLoad(t *testing.T) {
	t.Parallel()

	loader := MakeLoader(WithEnv(nil))
	handle := loader.Add[loaderConfig](WithName("confxloader"))

	if _, err := handle.Value(); !errors.Is(err, ErrNotLoaded) {
		t.Fatalf("got %v, want ErrNotLoaded", err)
	}
}

func TestAddAfterLoad(t *testing.T) {
	t.Parallel()

	env := map[string]string{
		"CONFXLOADER_HOST": "db",
	}

	loader := MakeLoader(WithEnv(env))
	first := loader.Add[loaderConfig](WithName("confxloader"))
	if err := loader.Load(); err != nil {
		t.Fatal(err)
	}

	late := loader.Add[loaderOtherConfig](WithName("confxother"))
	if _, err := late.Value(); !errors.Is(err, ErrRegisteredAfterLoad) {
		t.Fatalf("got %v, want ErrRegisteredAfterLoad", err)
	}

	// The late registration changes neither the result nor the loaded values.
	if err := loader.Load(); err != nil {
		t.Fatalf("a late registration changed the result: %v", err)
	}

	if cfg, err := first.Value(); err != nil || cfg.Host != "db" {
		t.Fatalf("got %+v, %v", cfg, err)
	}
}

func TestFailedLoadHandsOutNoValues(t *testing.T) {
	t.Parallel()

	env := map[string]string{
		"CONFXLOADER_HOST": "db",
		"CONFXOTHER_PORT":  "eighty",
	}

	loader := MakeLoader(WithEnv(env))
	good := loader.Add[loaderConfig](WithName("confxloader"))
	loader.Add[loaderOtherConfig](WithName("confxother"))

	err := loader.Load()
	if err == nil {
		t.Fatal("expected the load to fail")
	}

	cfg, valueErr := good.Value()
	if valueErr == nil || valueErr.Error() != err.Error() || cfg != (loaderConfig{}) {
		t.Fatalf("a config that loaded was handed out from a failed set: %+v, %v", cfg, valueErr)
	}
}

// TestLoadRunsOnce reads the process environment on purpose: a second Load must
// not take a new snapshot of it. Not parallel, for t.Setenv and the counter.
func TestLoadRunsOnce(t *testing.T) {
	t.Setenv("CONFXLOADER_HOST", "db")
	loaderDefaultCalls.Store(0)

	loader := MakeLoader()
	handle := loader.Add[loaderConfig](WithName("confxloader"))
	if err := loader.Load(); err != nil {
		t.Fatal(err)
	}

	t.Setenv("CONFXLOADER_HOST", "changed")
	if err := loader.Load(); err != nil {
		t.Fatal(err)
	}

	cfg, err := handle.Value()
	if err != nil || cfg.Host != "db" || loaderDefaultCalls.Load() != 1 {
		t.Fatalf("got %+v, %v, defaults calls %d", cfg, err, loaderDefaultCalls.Load())
	}
}

func TestLoaderIsSafeForConcurrentUse(t *testing.T) {
	t.Parallel()

	env := map[string]string{
		"CONFXLOADER_HOST": "db",
	}

	loader := MakeLoader(WithEnv(env))
	handle := loader.Add[loaderConfig](WithName("confxloader"))

	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			loader.Add[loaderOtherConfig](WithName("confxother"))
			_ = loader.Load()
			_, _ = handle.Value()
		})
	}

	wg.Wait()
}

func TestLoadReportsRegistrationsAndEveryConfig(t *testing.T) {
	t.Parallel()

	env := map[string]string{
		"CONFXLOADER_HSOT": "typo",
		"CONFXOTHER_PORT":  "eighty",
	}

	loader := MakeLoader(WithEnv(env))
	loader.Add[loaderConfig](WithName("confxloader"))
	loader.Add[loaderOtherConfig](WithName("confxother"))
	loader.Add[loaderOtherConfig]()
	loader.Add[loaderConfig](WithName("Invalid"))

	err := loader.Load()
	if err == nil {
		t.Fatal("expected the load to fail")
	}

	for _, want := range []string{
		`config confmaker.loaderOtherConfig has no instance name`,
		`instance name "Invalid"`,
		`unknown configuration variable "CONFXLOADER_HSOT" (did you mean "CONFXLOADER_HOST"?)`,
		`config "confxloader": required variable "CONFXLOADER_HOST" is not set`,
		`config "confxother": variable "CONFXOTHER_PORT"`,
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("report misses %q:\n%v", want, err)
		}
	}
}

// Not parallel: it counts SetDefaults calls in a shared counter.
func TestLoadCallsDefaultsOnceWithDump(t *testing.T) {
	loaderDefaultCalls.Store(0)

	var out bytes.Buffer
	env := WithEnv(map[string]string{"CONFXLOADER_HOST": "db"})
	cfg, err := Load[loaderConfig](WithDump(&out), env, WithName("confxloader"))
	if err != nil {
		t.Fatal(err)
	}

	if loaderDefaultCalls.Load() != 1 || cfg.Host != "db" || !strings.Contains(out.String(), "CONFXLOADER_HOST") {
		t.Fatalf("defaults calls %d, config %+v, dump:\n%s", loaderDefaultCalls.Load(), cfg, out.String())
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("disk full") }

func TestDumpFailureDoesNotHideTheReport(t *testing.T) {
	t.Parallel()

	env := map[string]string{
		"CONFXOTHER_PORT":   "eighty",
		"CONFXRENDER_VALUE": "1",
	}

	for name, dump := range map[string]EnvOption{
		"writer": WithDump(failingWriter{}),
		// brokenText loads but cannot render its default.
		"default": WithDump(&bytes.Buffer{}),
	} {
		t.Run(name, func(t *testing.T) {
			loader := MakeLoader(dump, WithEnv(env))
			loader.Add[loaderOtherConfig](WithName("confxother"))
			loader.Add[brokenTextConfig](WithName("confxrender"))

			err := loader.Load()
			if err == nil {
				t.Fatal("expected the load to fail")
			}

			want := `config "confxrender": field Value (variable "CONFXRENDER_VALUE"): cannot render default`
			if name == "writer" {
				want = "write configuration dump: disk full"
			}

			for _, part := range []string{want, `config "confxother": variable "CONFXOTHER_PORT"`} {
				if !strings.Contains(err.Error(), part) {
					t.Errorf("report misses %q:\n%v", part, err)
				}
			}
		})
	}
}

func TestDumpIsWrittenWhenLoadFails(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	if _, err := Load[loaderConfig](WithDump(&out), WithEnv(nil), WithName("confxloader")); err == nil {
		t.Fatal("expected the load to fail")
	}

	if !strings.Contains(out.String(), "CONFXLOADER_HOST") {
		t.Fatalf("dump not written:\n%s", out.String())
	}
}

func TestNilLoaderOptionIsReported(t *testing.T) {
	t.Parallel()

	if err := MakeLoader(nil, WithEnv(nil)).Load(); err == nil || !strings.Contains(err.Error(), "load option must not be nil") {
		t.Fatalf("got %v", err)
	}
}

func TestEmptyLoaderLoads(t *testing.T) {
	t.Parallel()

	if err := MakeLoader(WithEnv(nil)).Load(); err != nil {
		t.Fatal(err)
	}
}

// callbackConfig runs hook from its user methods, so a test can reach back into
// the loader that is loading it.
type callbackConfig struct {
	Host string `env:"HOST"`
}

var (
	callbackDefaults atomic.Pointer[func()]
	callbackValidate atomic.Pointer[func()]
)

func (*callbackConfig) SetDefaults() {
	if hook := callbackDefaults.Load(); hook != nil {
		(*hook)()
	}
}

func (callbackConfig) Validate() error {
	if hook := callbackValidate.Load(); hook != nil {
		(*hook)()
	}

	return nil
}

// withinDeadline fails the test instead of hanging it when run deadlocks.
func withinDeadline(t *testing.T, run func()) {
	t.Helper()

	finished := make(chan struct{})
	go func() {
		defer close(finished)
		run()
	}()

	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("the loader deadlocked")
	}
}

// waitForBlockedLoad returns once some goroutine is blocked in Loader.Load,
// waiting for a load another goroutine runs. It reads the goroutine stacks
// rather than sleeping, so the wait is proven, not assumed. Its callers are not
// parallel, so the blocked Load it finds is theirs.
func waitForBlockedLoad(t *testing.T) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	buf := make([]byte, 1<<20)
	for time.Now().Before(deadline) {
		stacks := string(buf[:runtime.Stack(buf, true)])
		for goroutine := range strings.SplitSeq(stacks, "\n\n") {
			if strings.Contains(goroutine, "[chan receive") && strings.Contains(goroutine, "confmaker.(*Loader).Load(") {
				return
			}
		}

		runtime.Gosched()
	}

	t.Fatal("no Load started waiting")
}

// Not parallel: the hooks are shared by every callbackConfig.
func TestUserCodeMayUseTheLoaderWhileItLoads(t *testing.T) {
	loader := MakeLoader(WithEnv(map[string]string{"CONFXCALLBACK_HOST": "db"}))
	handle := loader.Add[callbackConfig](WithName("confxcallback"))

	var valueErr, lateErr error
	validate := func() {
		_, valueErr = handle.Value()
		_, lateErr = loader.Add[loaderConfig](WithName("confxlate")).Value()
	}

	callbackValidate.Store(&validate)
	t.Cleanup(func() { callbackValidate.Store(nil) })

	withinDeadline(t, func() {
		if err := loader.Load(); err != nil {
			t.Error(err)
		}
	})

	if !errors.Is(valueErr, ErrNotLoaded) || !errors.Is(lateErr, ErrRegisteredAfterLoad) {
		t.Fatalf("during load: Value %v, late Add %v", valueErr, lateErr)
	}

	if cfg, err := handle.Value(); err != nil || cfg.Host != "db" {
		t.Fatalf("after load: %+v, %v", cfg, err)
	}
}

// Not parallel: the hooks are shared by every callbackConfig.
func TestConcurrentLoadWaitsForTheResult(t *testing.T) {
	loader := MakeLoader(WithEnv(map[string]string{"CONFXCALLBACK_HOST": "db"}))
	loader.Add[callbackConfig](WithName("confxcallback"))

	started, release := make(chan struct{}), make(chan struct{})
	defaults := func() {
		close(started)
		<-release
	}

	callbackDefaults.Store(&defaults)
	t.Cleanup(func() { callbackDefaults.Store(nil) })

	first := make(chan error)
	go func() { first <- loader.Load() }()
	<-started

	second := make(chan error)
	go func() { second <- loader.Load() }()
	waitForBlockedLoad(t)

	select {
	case err := <-second:
		t.Fatalf("a second Load returned before the first finished: %v", err)
	default:
	}

	close(release)
	withinDeadline(t, func() {
		if err1, err2 := <-first, <-second; err1 != nil || err2 != nil {
			t.Errorf("results: %v, %v", err1, err2)
		}
	})
}

// Not parallel: the hooks are shared by every callbackConfig.
func TestPanicDuringLoadFinishesTheLoad(t *testing.T) {
	loader := MakeLoader(WithEnv(nil))
	handle := loader.Add[callbackConfig](WithName("confxcallback"))

	started, release := make(chan struct{}), make(chan struct{})
	defaults := func() {
		close(started)
		<-release
		panic("defaults exploded")
	}

	callbackDefaults.Store(&defaults)
	t.Cleanup(func() { callbackDefaults.Store(nil) })

	recovered := make(chan any)
	go func() {
		defer func() { recovered <- recover() }()
		_ = loader.Load()
	}()

	<-started

	// This Load is already waiting when the panic happens.
	waiting := make(chan error)
	go func() { waiting <- loader.Load() }()
	waitForBlockedLoad(t)
	close(release)

	withinDeadline(t, func() {
		if <-recovered == nil {
			t.Error("the panic did not reach the Load that ran it")
		}

		if err := <-waiting; !errors.Is(err, errLoadPanicked) {
			t.Errorf("waiting Load: %v", err)
		}

		if err := loader.Load(); !errors.Is(err, errLoadPanicked) {
			t.Errorf("Load after the panic: %v", err)
		}

		if _, err := handle.Value(); !errors.Is(err, errLoadPanicked) {
			t.Errorf("Value after the panic: %v", err)
		}
	})
}

// Not parallel: the hooks are shared by every callbackConfig.
func TestConflictStopsLoadingBeforeUserCode(t *testing.T) {
	calls := 0
	defaults := func() { calls++ }
	callbackDefaults.Store(&defaults)
	t.Cleanup(func() { callbackDefaults.Store(nil) })

	var out bytes.Buffer
	loader := MakeLoader(WithEnv(nil), WithDump(&out))
	loader.Add[callbackConfig](WithName("confxsame"))
	loader.Add[callbackConfig](WithName("confxsame"))

	err := loader.Load()
	if err == nil || !strings.Contains(err.Error(), "registered more than once") {
		t.Fatalf("got %v", err)
	}

	if calls != 0 || out.Len() != 0 {
		t.Fatalf("user code ran for a set that cannot load: defaults calls %d, dump %q", calls, out.String())
	}
}

func TestAllowUnknownCopiesItsPrefixes(t *testing.T) {
	t.Parallel()

	prefixes := []string{"CONFXOTHER_"}
	option := AllowUnknown(prefixes...)
	prefixes[0] = "CONFXLOADER_"

	env := map[string]string{"CONFXLOADER_HOST": "db", "CONFXLOADER_HSOT": "typo"}
	_, err := Load[loaderConfig](WithName("confxloader"), WithEnv(env), option)
	if err == nil || !strings.Contains(err.Error(), "CONFXLOADER_HSOT") {
		t.Fatalf("a change to the caller's slice reached the option: %v", err)
	}
}

type lateNamedConfig struct {
	Host string `env:"HOST"`
}

func (lateNamedConfig) ConfigName() string { panic("ConfigName ran for a late registration") }

func TestLateAddRunsNoUserCode(t *testing.T) {
	t.Parallel()

	loader := MakeLoader(WithEnv(nil))
	if err := loader.Load(); err != nil {
		t.Fatal(err)
	}

	if _, err := loader.Add[lateNamedConfig]().Value(); !errors.Is(err, ErrRegisteredAfterLoad) {
		t.Fatalf("got %v", err)
	}
}

func TestConflictReportDoesNotDependOnAddOrder(t *testing.T) {
	t.Parallel()

	var reports []string
	for _, prefixes := range [][]string{{"CONFXA_", "CONFXB_", "CONFXA_", "CONFXB_"}, {"CONFXB_", "CONFXA_", "CONFXB_", "CONFXA_"}} {
		loader := MakeLoader(WithEnv(nil))
		for _, prefix := range prefixes {
			loader.Add[loaderConfig](WithName("confxsame"), WithPrefix(prefix))
		}

		reports = append(reports, loader.Load().Error())
	}

	if reports[0] != reports[1] {
		t.Fatalf("reports differ:\n%s\n---\n%s", reports[0], reports[1])
	}
}

func TestInvalidRegistrationDoesNotHideOtherConfigs(t *testing.T) {
	t.Parallel()

	loader := MakeLoader(WithEnv(nil))
	loader.Add[loaderConfig]()
	loader.Add[loaderOtherConfig](WithName("confxother"))

	err := loader.Load()
	for _, want := range []string{"has no instance name", `required variable "CONFXOTHER_PORT" is not set`} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("report misses %q: %v", want, err)
		}
	}
}

func TestReportDoesNotDependOnAddOrder(t *testing.T) {
	t.Parallel()

	adds := []func(*Loader){
		func(l *Loader) { l.Add[loaderOtherConfig]() },
		func(l *Loader) { l.Add[loaderConfig]() },
		func(l *Loader) { l.Add[loaderConfig](WithName("Invalid")) },
		func(l *Loader) { l.Add[loaderConfig](nil) },
		func(l *Loader) { l.Add[loaderOtherConfig](WithName("confxother")) },
	}

	var reports []string
	for _, order := range [][]int{{0, 1, 2, 3, 4}, {4, 3, 2, 1, 0}, {2, 0, 4, 1, 3}} {
		loader := MakeLoader(WithEnv(nil))
		for _, i := range order {
			adds[i](loader)
		}

		reports = append(reports, loader.Load().Error())
	}

	for _, report := range reports[1:] {
		if report != reports[0] {
			t.Fatalf("reports differ:\n%s\n---\n%s", reports[0], report)
		}
	}
}

func TestDumpDoesNotDependOnAddOrder(t *testing.T) {
	t.Parallel()

	env := WithEnv(map[string]string{"CONFXLOADER_HOST": "db", "CONFXOTHER_PORT": "5432"})

	var dumps []string
	for _, reversed := range []bool{false, true} {
		var out bytes.Buffer
		loader := MakeLoader(env, WithDump(&out))
		adds := []func(){
			func() { loader.Add[loaderConfig](WithName("confxloader")) },
			func() { loader.Add[loaderOtherConfig](WithName("confxother")) },
		}

		if reversed {
			slices.Reverse(adds)
		}

		for _, add := range adds {
			add()
		}

		if err := loader.Load(); err != nil {
			t.Fatal(err)
		}

		dumps = append(dumps, out.String())
	}

	if dumps[0] != dumps[1] || strings.Index(dumps[0], "confxloader") > strings.Index(dumps[0], "confxother") {
		t.Fatalf("dumps differ or are not sorted:\n%s\n---\n%s", dumps[0], dumps[1])
	}
}

func TestZeroHandle(t *testing.T) {
	t.Parallel()

	var handle Handle[loaderConfig]
	if _, err := handle.Value(); err == nil || !strings.Contains(err.Error(), "belongs to no loader") {
		t.Fatalf("got %v", err)
	}
}

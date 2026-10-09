package confmaker

import (
	"errors"
	"runtime"
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
	handle := loader.Register[loaderConfig]("confxloader")

	if _, err := handle.Value(); !errors.Is(err, ErrNotLoaded) {
		t.Fatalf("got %v, want ErrNotLoaded", err)
	}
}

func TestRegisterAfterLoad(t *testing.T) {
	t.Parallel()

	env := map[string]string{
		"CONFXLOADER_HOST": "db",
	}

	loader := MakeLoader(WithEnv(env))
	first := loader.Register[loaderConfig]("confxloader")
	if err := loader.Load(); err != nil {
		t.Fatal(err)
	}

	late := loader.Register[loaderOtherConfig]("confxother")
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
	good := loader.Register[loaderConfig]("confxloader")
	loader.Register[loaderOtherConfig]("confxother")

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
	handle := loader.Register[loaderConfig]("confxloader")
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
	handle := loader.Register[loaderConfig]("confxloader")

	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			loader.Register[loaderOtherConfig]("confxother")
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
	loader.Register[loaderConfig]("confxloader")
	loader.Register[loaderOtherConfig]("confxother")
	loader.Register[loaderOtherConfig]("")
	loader.Register[loaderConfig]("Invalid")

	err := loader.Load()
	if err == nil {
		t.Fatal("expected the load to fail")
	}

	for _, want := range []string{
		`an instance name is required`,
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
			if strings.Contains(goroutine, "[chan receive") && strings.Contains(goroutine, "confmaker/v2.(*Loader).Load(") {
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
	handle := loader.Register[callbackConfig]("confxcallback")

	var valueErr, lateErr error
	validate := func() {
		_, valueErr = handle.Value()
		_, lateErr = loader.Register[loaderConfig]("confxlate").Value()
	}

	callbackValidate.Store(&validate)
	t.Cleanup(func() { callbackValidate.Store(nil) })

	withinDeadline(t, func() {
		if err := loader.Load(); err != nil {
			t.Error(err)
		}
	})

	if !errors.Is(valueErr, ErrNotLoaded) || !errors.Is(lateErr, ErrRegisteredAfterLoad) {
		t.Fatalf("during load: Value %v, late Register %v", valueErr, lateErr)
	}

	if cfg, err := handle.Value(); err != nil || cfg.Host != "db" {
		t.Fatalf("after load: %+v, %v", cfg, err)
	}
}

// Not parallel: the hooks are shared by every callbackConfig.
func TestConcurrentLoadWaitsForTheResult(t *testing.T) {
	loader := MakeLoader(WithEnv(map[string]string{"CONFXCALLBACK_HOST": "db"}))
	loader.Register[callbackConfig]("confxcallback")

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
	handle := loader.Register[callbackConfig]("confxcallback")

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

		if err := <-waiting; !errors.Is(err, ErrLoadPanicked) {
			t.Errorf("waiting Load: %v", err)
		}

		if err := loader.Load(); !errors.Is(err, ErrLoadPanicked) {
			t.Errorf("Load after the panic: %v", err)
		}

		if _, err := handle.Value(); !errors.Is(err, ErrLoadPanicked) {
			t.Errorf("Value after the panic: %v", err)
		}
	})
}

func TestAllowUnknownCopiesItsPrefixes(t *testing.T) {
	t.Parallel()

	prefixes := []string{"CONFXOTHER_"}
	option := AllowUnknown(prefixes...)
	prefixes[0] = "CONFXLOADER_"

	env := map[string]string{"CONFXLOADER_HOST": "db", "CONFXLOADER_HSOT": "typo"}
	_, err := Load[loaderConfig]("confxloader", WithEnv(env), option)
	if err == nil || !strings.Contains(err.Error(), "CONFXLOADER_HSOT") {
		t.Fatalf("a change to the caller's slice reached the option: %v", err)
	}
}

type lateConfig struct {
	Host string `env:"HOST"`
}

func (*lateConfig) SetDefaults() { panic("SetDefaults ran for a late registration") }

func TestLateRegisterRunsNoUserCode(t *testing.T) {
	t.Parallel()

	loader := MakeLoader(WithEnv(nil))
	if err := loader.Load(); err != nil {
		t.Fatal(err)
	}

	if _, err := loader.Register[lateConfig]("late").Value(); !errors.Is(err, ErrRegisteredAfterLoad) {
		t.Fatalf("got %v", err)
	}
}

func TestConflictReportDoesNotDependOnRegistrationOrder(t *testing.T) {
	t.Parallel()

	var reports []string
	for _, prefixes := range [][]string{{"CONFXA_", "CONFXB_", "CONFXA_", "CONFXB_"}, {"CONFXB_", "CONFXA_", "CONFXB_", "CONFXA_"}} {
		loader := MakeLoader(WithEnv(nil))
		for _, prefix := range prefixes {
			loader.Register[loaderConfig]("confxsame", WithPrefix(prefix))
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
	loader.Register[loaderConfig]("")
	loader.Register[loaderOtherConfig]("confxother")

	err := loader.Load()
	for _, want := range []string{"an instance name is required", `required variable "CONFXOTHER_PORT" is not set`} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("report misses %q: %v", want, err)
		}
	}
}

func TestReportDoesNotDependOnRegistrationOrder(t *testing.T) {
	t.Parallel()

	adds := []func(*Loader){
		func(l *Loader) { l.Register[loaderOtherConfig]("") },
		func(l *Loader) { l.Register[loaderConfig]("") },
		func(l *Loader) { l.Register[loaderConfig]("Invalid") },
		func(l *Loader) { l.Register[loaderConfig]("app", nil) },
		func(l *Loader) { l.Register[loaderOtherConfig]("confxother") },
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

func TestZeroHandle(t *testing.T) {
	t.Parallel()

	var handle Handle[loaderConfig]
	if _, err := handle.Value(); err == nil || !strings.Contains(err.Error(), "belongs to no loader") {
		t.Fatalf("got %v", err)
	}
}

func TestCopiedHandleSharesTypedResult(t *testing.T) {
	type config struct {
		Host string `env:"HOST"`
	}
	loader := MakeLoader(WithEnv(map[string]string{"APP_HOST": "database"}))
	handle := loader.Register[config]("app")
	copied := *handle
	if _, err := copied.Value(); !errors.Is(err, ErrNotLoaded) {
		t.Fatalf("before Load: %v", err)
	}

	if err := loader.Load(); err != nil {
		t.Fatal(err)
	}

	for _, result := range []*Handle[config]{handle, &copied} {
		value, err := result.Value()
		if err != nil || value.Host != "database" {
			t.Fatalf("after Load: %+v, %v", value, err)
		}
	}
}

func TestHandleRegistrationMetadata(t *testing.T) {
	loader := MakeLoader(WithEnv(nil))
	handle := loader.Register[struct{}]("replica", WithPrefix("READ_"))
	if handle.Name() != "replica" || !handle.BelongsTo(loader) || handle.BelongsTo(MakeLoader()) || handle.BelongsTo(nil) {
		t.Fatal("incorrect metadata")
	}

	var nilHandle *Handle[struct{}]
	if len(nilHandle.Name()) != 0 || nilHandle.BelongsTo(loader) {
		t.Fatal("nil metadata")
	}

	zero := new(Handle[struct{}])
	if len(zero.Name()) != 0 || zero.BelongsTo(loader) {
		t.Fatal("zero metadata")
	}

	if err := loader.Load(); err != nil {
		t.Fatal(err)
	}

	late := loader.Register[struct{}]("late")
	if late.BelongsTo(loader) {
		t.Fatal("late handle must not appear registered")
	}
}

func TestConflictStopsLoadingBeforeDefaults(t *testing.T) {
	loaderDefaultCalls.Store(0)
	loader := MakeLoader(WithEnv(nil))
	loader.Register[loaderConfig]("app")
	loader.Register[loaderConfig]("app")

	if err := loader.Load(); err == nil {
		t.Fatal("expected conflict")
	}

	if loaderDefaultCalls.Load() != 0 {
		t.Fatal("conflict evaluated defaults")
	}
}

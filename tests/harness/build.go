package harness

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

var (
	buildOnce sync.Once
	builtBin  string
	buildErr  error
)

// BuildBinary returns the path to an `hc` binary built from the current branch.
//
// If HC_E2E_BIN points at an existing file (set by `make e2e-migration` which
// builds once), that binary is reused. Otherwise the binary is built lazily and
// cached for the lifetime of the test process: mock fixtures are generated
// first (so RAW/DEBIAN bytes are embedded) and then `go build ./cmd/hc` runs.
func BuildBinary(t *testing.T) string {
	t.Helper()

	if pre := os.Getenv("HC_E2E_BIN"); pre != "" {
		if _, err := os.Stat(pre); err == nil {
			return pre
		}
		t.Fatalf("HC_E2E_BIN=%q does not exist", pre)
	}

	buildOnce.Do(func() {
		builtBin, buildErr = buildHC()
	})
	if buildErr != nil {
		t.Fatalf("failed to build hc binary: %v", buildErr)
	}
	return builtBin
}

func buildHC() (string, error) {
	root, err := repoRoot()
	if err != nil {
		return "", err
	}

	// Generate mock binary fixtures so they are embedded into the binary.
	if out, err := runAt(root, "go", "run", "./module/ar/migrate/adapter/mock_jfrog/cmd"); err != nil {
		return "", wrap("mock-init failed", out, err)
	}

	outDir, err := os.MkdirTemp("", "hc-e2e-bin-")
	if err != nil {
		return "", err
	}
	bin := filepath.Join(outDir, "hc")

	if out, err := runAt(root, "go", "build", "-o", bin, "./cmd/hc"); err != nil {
		return "", wrap("go build failed", out, err)
	}
	return bin, nil
}

func runAt(dir, name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	return cmd.CombinedOutput()
}

func wrap(msg string, out []byte, err error) error {
	return &buildError{msg: msg, out: string(out), err: err}
}

type buildError struct {
	msg string
	out string
	err error
}

func (e *buildError) Error() string {
	return e.msg + ": " + e.err.Error() + "\n" + e.out
}

// repoRoot walks up from this source file until it finds the module's go.mod.
func repoRoot() (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", errNoCaller
	}
	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errNoGoMod
		}
		dir = parent
	}
}

var (
	errNoCaller = &staticError{"cannot determine caller path for repo root"}
	errNoGoMod  = &staticError{"could not locate go.mod above test harness"}
)

type staticError struct{ s string }

func (e *staticError) Error() string { return e.s }

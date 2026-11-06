package apptest

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
)

// testBinaryPath is the mdui binary built once for the whole test run. It is
// set by TestMain before any test executes.
var (
	testBinaryOnce sync.Once
	testBinaryPath string
	testBinaryErr  error
)

// BuildBinary returns the path of the mdui binary built from the repo root,
// building it on first use. A build failure fails the calling test.
func BuildBinary(t *testing.T) string {
	t.Helper()
	testBinaryOnce.Do(func() {
		dir, err := os.MkdirTemp("", "mdui-apptest-bin")
		if err != nil {
			testBinaryErr = err
			return
		}
		bin := filepath.Join(dir, "mdui-under-test")
		build := exec.Command("go", "build", "-o", bin, ".")
		build.Dir = repoRoot()
		if out, err := build.CombinedOutput(); err != nil {
			os.RemoveAll(dir)
			testBinaryErr = fmt.Errorf("go build mdui: %w\n%s", err, out)
			return
		}
		testBinaryPath = bin
	})
	if testBinaryErr != nil {
		t.Fatalf("building the mdui binary: %v", testBinaryErr)
	}
	return testBinaryPath
}

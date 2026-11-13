package main

import (
	"os"
	"path/filepath"
	"testing"
)

// cgroupTree writes the given contents to v2 and v1 files under a temp dir. An
// empty string leaves that file missing.
func cgroupTree(t *testing.T, v2, v1 string) cgroupFiles {
	t.Helper()
	dir := t.TempDir()
	files := cgroupFiles{
		v2: filepath.Join(dir, "memory.max"),
		v1: filepath.Join(dir, "memory.limit_in_bytes"),
	}
	if v2 != "" {
		if err := os.WriteFile(files.v2, []byte(v2), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if v1 != "" {
		if err := os.WriteFile(files.v1, []byte(v1), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return files
}

func TestCgroupMemoryLimit(t *testing.T) {
	const gib = 1 << 30
	tests := []struct {
		name   string
		v2, v1 string
		want   int64
		ok     bool
	}{
		{"v2 number", "1073741824\n", "", gib, true},
		{"v2 max means no limit", "max\n", "", 0, false},
		{"v1 number", "", "1073741824\n", gib, true},
		{"v1 huge means no limit", "", "9223372036854775807\n", 0, false},
		{"v1 just above 1<<60 means no limit", "", "1152921504606846977", 0, false},
		{"v1 exactly 1<<60 is a limit", "", "1152921504606846976", 1 << 60, true},
		{"v2 max wins over a v1 number", "max", "1073741824", 0, false},
		{"both missing", "", "", 0, false},
		{"v2 garbage falls back to v1", "lots", "1073741824", gib, true},
		{"garbage", "not a number", "also not", 0, false},
		{"zero", "0", "", 0, false},
		{"negative", "-5", "", 0, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := cgroupMemoryLimit(cgroupTree(t, tc.v2, tc.v1))
			if ok != tc.ok || got != tc.want {
				t.Errorf("cgroupMemoryLimit = (%d, %v), want (%d, %v)", got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestCgroupMemoryLimitAbsentFiles(t *testing.T) {
	files := cgroupFiles{
		v2: filepath.Join(t.TempDir(), "nope", "memory.max"),
		v1: filepath.Join(t.TempDir(), "nope", "memory.limit_in_bytes"),
	}
	if got, ok := cgroupMemoryLimit(files); ok || got != 0 {
		t.Errorf("absent files: got (%d, %v), want (0, false)", got, ok)
	}
}

// useSetMemLimit records the soft limits applyMemoryLimit sets.
func useSetMemLimit(t *testing.T) *[]int64 {
	t.Helper()
	var calls []int64
	old := setMemLimit
	setMemLimit = func(n int64) int64 {
		calls = append(calls, n)
		return 0
	}
	t.Cleanup(func() { setMemLimit = old })
	return &calls
}

func noEnv(string) string { return "" }

func TestApplyMemoryLimitSetsThreeQuarters(t *testing.T) {
	calls := useSetMemLimit(t)
	files := cgroupTree(t, "1073741824", "")
	if got := applyMemoryLimit(noEnv, files); got != 1<<30 {
		t.Fatalf("returned limit = %d, want %d", got, 1<<30)
	}
	if len(*calls) != 1 || (*calls)[0] != (1<<30)*3/4 {
		t.Errorf("SetMemoryLimit calls = %v, want one call with %d", *calls, (1<<30)*3/4)
	}
}

func TestApplyMemoryLimitRespectsGOMEMLIMIT(t *testing.T) {
	calls := useSetMemLimit(t)
	files := cgroupTree(t, "1073741824", "")
	env := func(k string) string {
		if k == "GOMEMLIMIT" {
			return "512MiB"
		}
		return ""
	}
	if got := applyMemoryLimit(env, files); got != 1<<30 {
		t.Errorf("returned limit = %d, want the container limit %d", got, 1<<30)
	}
	if len(*calls) != 0 {
		t.Errorf("SetMemoryLimit called with GOMEMLIMIT set: %v", *calls)
	}
}

func TestApplyMemoryLimitNoContainer(t *testing.T) {
	calls := useSetMemLimit(t)
	if got := applyMemoryLimit(noEnv, cgroupTree(t, "max", "")); got != 0 {
		t.Errorf("returned limit = %d, want 0", got)
	}
	if len(*calls) != 0 {
		t.Errorf("SetMemoryLimit called without a container limit: %v", *calls)
	}
}

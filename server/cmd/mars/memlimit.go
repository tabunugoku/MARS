package main

import (
	"log/slog"
	"os"
	"runtime/debug"
	"strconv"
	"strings"
)

// cgroupFiles are the memory limit files of cgroup v2 and v1. On a host
// without them (Windows, macOS, a bare host) nothing is found and the Go
// runtime keeps its defaults.
type cgroupFiles struct {
	v2, v1 string
}

var defaultCgroupFiles = cgroupFiles{
	v2: "/sys/fs/cgroup/memory.max",
	v1: "/sys/fs/cgroup/memory/memory.limit_in_bytes",
}

// setMemLimit is debug.SetMemoryLimit. Tests replace it.
var setMemLimit = debug.SetMemoryLimit

// cgroupMemoryLimit returns the container's memory limit in bytes. A v2 "max"
// means no limit. A value that is missing, not a number, or 1<<60 or more
// (v1's "unlimited") is no limit either; v1 is tried when v2 gives none.
func cgroupMemoryLimit(files cgroupFiles) (int64, bool) {
	if b, err := os.ReadFile(files.v2); err == nil {
		s := strings.TrimSpace(string(b))
		if s == "max" {
			return 0, false
		}
		if n, ok := parseLimit(s); ok {
			return n, true
		}
	}
	if b, err := os.ReadFile(files.v1); err == nil {
		if n, ok := parseLimit(strings.TrimSpace(string(b))); ok {
			return n, true
		}
	}
	return 0, false
}

// parseLimit reads one cgroup limit value. It reports false for garbage, zero,
// negative and "no limit" values.
func parseLimit(s string) (int64, bool) {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n <= 0 || n > 1<<60 {
		return 0, false
	}
	return n, true
}

// applyMemoryLimit finds the container's memory limit and returns it (0 when
// there is none). Unless GOMEMLIMIT is set, it also sets the Go soft memory
// limit to three quarters of it, so the heap is collected before the kernel
// has to kill the process. The returned limit sizes the decode budget even when
// GOMEMLIMIT is set.
func applyMemoryLimit(getenv func(string) string, files cgroupFiles) int64 {
	limit, ok := cgroupMemoryLimit(files)
	if !ok {
		return 0
	}
	if getenv("GOMEMLIMIT") != "" {
		slog.Info("memory limit", "container", limit, "go", "GOMEMLIMIT")
		return limit
	}
	soft := limit * 3 / 4
	setMemLimit(soft)
	slog.Info("memory limit", "container", limit, "go", soft)
	return limit
}

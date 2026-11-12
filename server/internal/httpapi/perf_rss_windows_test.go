//go:build perf && windows

package httpapi

import (
	"syscall"
	"unsafe"
)

// peakRSS is the process's peak working set (PROCESS_MEMORY_COUNTERS).
func peakRSS() int64 {
	var c struct {
		cb                                                 uint32
		pageFaultCount                                     uint32
		peakWorkingSetSize, workingSetSize                 uintptr
		quotaPeakPagedPoolUsage, quotaPagedPoolUsage       uintptr
		quotaPeakNonPagedPoolUsage, quotaNonPagedPoolUsage uintptr
		pagefileUsage, peakPagefileUsage                   uintptr
	}
	c.cb = uint32(unsafe.Sizeof(c))
	h, _ := syscall.GetCurrentProcess()
	proc := syscall.NewLazyDLL("psapi.dll").NewProc("GetProcessMemoryInfo")
	if r, _, _ := proc.Call(uintptr(h), uintptr(unsafe.Pointer(&c)), uintptr(c.cb)); r == 0 {
		return 0
	}
	return int64(c.peakWorkingSetSize)
}

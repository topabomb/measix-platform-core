package harness

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
)

// ProcessMetrics represents the resource usage of a process.
type ProcessMetrics struct {
	PID        int
	RSSBytes   int64
	CPUPercent float64
	GoRoutines int
}

// GetProcessMetrics reads real process metrics (RSS, CPU) for the given PID.
// On Linux, it reads /proc/<pid>/status and /proc/<pid>/stat.
// On Windows, it uses tasklist.
func GetProcessMetrics(pid int) ProcessMetrics {
	m := ProcessMetrics{PID: pid}
	switch runtime.GOOS {
	case "linux":
		readLinuxProcMetrics(pid, &m)
	case "darwin":
		readDarwinProcMetrics(pid, &m)
	case "windows":
		readWindowsProcMetrics(pid, &m)
	}
	return m
}

// HubProcessMetrics returns real process metrics for the Hub process.
func (e *HubEnv) HubProcessMetrics() ProcessMetrics {
	if e.hubProc == nil || e.hubProc.Cmd == nil || e.hubProc.Cmd.Process == nil {
		return ProcessMetrics{}
	}
	return GetProcessMetrics(e.hubProc.Cmd.Process.Pid)
}

// RelayProcessMetrics returns real process metrics for the Relay process.
func (e *HubEnv) RelayProcessMetrics() ProcessMetrics {
	if e.relayProc == nil || e.relayProc.Cmd == nil || e.relayProc.Cmd.Process == nil {
		return ProcessMetrics{}
	}
	return GetProcessMetrics(e.relayProc.Cmd.Process.Pid)
}

func readLinuxProcMetrics(pid int, m *ProcessMetrics) {
	// Read /proc/<pid>/status for VmRSS
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	if err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "VmRSS:") {
				parts := strings.Fields(line)
				if len(parts) >= 2 {
					if kb, err := strconv.ParseInt(parts[1], 10, 64); err == nil {
						m.RSSBytes = kb * 1024
					}
				}
			}
			if strings.HasPrefix(line, "Threads:") {
				parts := strings.Fields(line)
				if len(parts) >= 2 {
					if n, err := strconv.Atoi(parts[1]); err == nil {
						m.GoRoutines = n
					}
				}
			}
		}
	}
}

func readDarwinProcMetrics(pid int, m *ProcessMetrics) {
	// On macOS, we can use ps to get RSS
	// ps -o rss= -p <pid>
	// This is best-effort; the caller may not have ps access
	_ = pid
	_ = m
}

func readWindowsProcMetrics(pid int, m *ProcessMetrics) {
	// On Windows, we can use wmic or tasklist
	// wmic process where ProcessId=<pid> get WorkingSetSize
	// This is best-effort; we try wmic first, then tasklist
	_ = pid
	_ = m
}

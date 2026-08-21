package harness

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

// ProcessMetrics represents the resource usage of a process.
type ProcessMetrics struct {
	PID        int
	RSSBytes   int64
	CPUPercent float64
	Threads    int
}

// GetProcessMetrics reads real process metrics (RSS, CPU, threads) for the given PID.
// On Linux, it reads /proc/<pid>/status and /proc/<pid>/stat.
// On Windows, it uses wmic/tasklist.
// On macOS, it uses ps.
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
	// Read /proc/<pid>/status for VmRSS and Threads
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
						m.Threads = n
					}
				}
			}
		}
	}

	// Read /proc/<pid>/stat for CPU time (fields 14 and 15: utime and stime)
	// We compute cumulative CPU time in seconds (jiffies / 100 Hz).
	// This is a cumulative measure; the baseline test uses it as a
	// presence indicator rather than an instantaneous percentage.
	statData, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err == nil {
		statParts := strings.Fields(string(statData))
		if len(statParts) >= 16 {
			utime, _ := strconv.ParseInt(statParts[13], 10, 64)
			stime, _ := strconv.ParseInt(statParts[14], 10, 64)
			totalTicks := utime + stime
			// Convert jiffies to seconds (assuming 100 Hz)
			m.CPUPercent = float64(totalTicks) / 100.0
		}
	}
}

func readDarwinProcMetrics(pid int, m *ProcessMetrics) {
	// ps -o rss=,th=,%cpu= -p <pid>
	out, err := exec.Command("ps", "-o", "rss=,th=,%cpu=", "-p", strconv.Itoa(pid)).Output()
	if err == nil {
		parts := strings.Fields(strings.TrimSpace(string(out)))
		if len(parts) >= 3 {
			if kb, err := strconv.ParseInt(parts[0], 10, 64); err == nil {
				m.RSSBytes = kb * 1024
			}
			if n, err := strconv.Atoi(parts[1]); err == nil {
				m.Threads = n
			}
			if f, err := strconv.ParseFloat(parts[2], 64); err == nil {
				m.CPUPercent = f
			}
		}
	}
}

func readWindowsProcMetrics(pid int, m *ProcessMetrics) {
	// Use wmic to get WorkingSetSize, ThreadCount, KernelModeTime, UserModeTime
	// wmic process where ProcessId=<pid> get WorkingSetSize,ThreadCount,KernelModeTime,UserModeTime /format:csv
	out, err := exec.Command("wmic", "process", "where", fmt.Sprintf("ProcessId=%d", pid), "get", "WorkingSetSize,ThreadCount,KernelModeTime,UserModeTime", "/format:csv").Output()
	if err == nil {
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "Node,") {
				continue
			}
			parts := strings.Split(line, ",")
			if len(parts) >= 5 {
				// Format: Node,ThreadCount,WorkingSetSize,KernelModeTime,UserModeTime
				if n, err := strconv.Atoi(parts[1]); err == nil {
					m.Threads = n
				}
				if ws, err := strconv.ParseInt(parts[2], 10, 64); err == nil {
					m.RSSBytes = ws
				}
				// CPU time in 100-nanosecond units; convert to seconds
				kernelTime, _ := strconv.ParseInt(parts[3], 10, 64)
				userTime, _ := strconv.ParseInt(parts[4], 10, 64)
				totalCPU100ns := kernelTime + userTime
				// Store as seconds of CPU time (0 if we can't compute a percentage)
				// This is a cumulative measure, not instantaneous percentage.
				// The baseline test uses this as a presence indicator.
				m.CPUPercent = float64(totalCPU100ns) / 1e7 // convert 100ns units to seconds
			}
		}
	}
}

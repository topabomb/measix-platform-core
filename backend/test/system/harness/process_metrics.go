package harness

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// ProcessMetrics represents the resource usage of a process.
type ProcessMetrics struct {
	PID        int
	RSSBytes   int64
	CPUPercent float64 // true interval-delta CPU percentage (0–100*numCores)
	Threads    int
	// CumulativeCPUSeconds is the total CPU time consumed by the process
	// (user + kernel) since it started. This is a diagnostic value, not
	// a percentage.
	CumulativeCPUSeconds float64
}

// CPUSnapshot captures a point-in-time reading of process CPU usage
// for computing interval-delta CPU percentage.
type CPUSnapshot struct {
	TotalCPUSeconds float64
	WallTime        time.Time
}

// GetProcessMetrics reads real process metrics (RSS, CPU, threads) for the given PID.
//
// CPUPercent is computed as a true interval-delta percentage:
//
//	ΔprocessCPU / ΔwallTime / numCores * 100
//
// On Linux: reads /proc/<pid>/stat for utime+stime (jiffies → seconds).
// On Windows: reads KernelModeTime+UserModeTime (100ns units → seconds).
// On macOS: reads ps %cpu directly (already an interval percentage).
//
// The first call returns CPUPercent=0 (no previous snapshot).
// Subsequent calls compute the delta from the previous snapshot.
var cpuSnapshots = map[int]CPUSnapshot{}

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

// computeCPUPercent calculates the true CPU percentage from cumulative
// CPU time snapshots. Returns 0 if the interval is too small or if this
// is the first measurement (no previous snapshot).
func computeCPUPercent(pid int, currentCPUSeconds float64) float64 {
	now := time.Now()
	prev, hasPrev := cpuSnapshots[pid]
	cpuSnapshots[pid] = CPUSnapshot{TotalCPUSeconds: currentCPUSeconds, WallTime: now}

	if !hasPrev {
		return 0
	}

	deltaCPU := currentCPUSeconds - prev.TotalCPUSeconds
	deltaWall := now.Sub(prev.WallTime).Seconds()
	if deltaWall <= 0 {
		return 0
	}

	numCores := float64(runtime.NumCPU())
	if numCores <= 0 {
		numCores = 1
	}

	// CPU percentage: (deltaCPU / deltaWall) / numCores * 100
	// This gives a value in [0, 100*numCores] range.
	percent := (deltaCPU / deltaWall) / numCores * 100
	if percent < 0 {
		percent = 0
	}
	return percent
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
	// These are cumulative jiffies (typically 100 Hz).
	// We convert to seconds and use computeCPUPercent for interval-delta.
	statData, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err == nil {
		statParts := strings.Fields(string(statData))
		if len(statParts) >= 16 {
			utime, _ := strconv.ParseInt(statParts[13], 10, 64)
			stime, _ := strconv.ParseInt(statParts[14], 10, 64)
			totalTicks := utime + stime
			// Convert jiffies to seconds (assuming 100 Hz on most Linux systems)
			m.CumulativeCPUSeconds = float64(totalTicks) / 100.0
			m.CPUPercent = computeCPUPercent(pid, m.CumulativeCPUSeconds)
		}
	}
}

func readDarwinProcMetrics(pid int, m *ProcessMetrics) {
	// ps -o rss=,th=,%cpu= -p <pid>
	// On macOS, ps %cpu is already an interval percentage.
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
				m.CumulativeCPUSeconds = float64(totalCPU100ns) / 1e7
				m.CPUPercent = computeCPUPercent(pid, m.CumulativeCPUSeconds)
			}
		}
	}
}

package metrics

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// SystemMetrics holds system resource usage data
type SystemMetrics struct {
	CPUUsage  float64 `json:"cpu_usage"`  // 0-100%
	RAMUsage  float64 `json:"ram_usage"`  // 0-100%
	DiskUsage float64 `json:"disk_usage"` // 0-100%
	Uptime    int64   `json:"uptime"`     // seconds
	LastBuild int64   `json:"last_build"` // Unix timestamp of last success build
}

var (
	startTime    = time.Now()
	prevCPUIdle  uint64
	prevCPUTotal uint64
)

// Collect gathers current system metrics
// Uses native Linux /proc filesystem for cross-distro compatibility
func Collect(projectDir string) (*SystemMetrics, error) {
	m := &SystemMetrics{
		Uptime: int64(time.Since(startTime).Seconds()),
	}

	// Read last build time from file
	if projectDir != "" {
		if content, err := os.ReadFile(filepath.Join(projectDir, ".last_build")); err == nil {
			if ts, err := strconv.ParseInt(strings.TrimSpace(string(content)), 10, 64); err == nil {
				m.LastBuild = ts
			}
		}
	}

	// CPU Usage
	cpu, err := getCPUUsage()
	if err == nil {
		m.CPUUsage = cpu
	}

	// RAM Usage
	ram, err := getRAMUsage()
	if err == nil {
		m.RAMUsage = ram
	}

	// Disk Usage
	disk, err := getDiskUsage("/")
	if err == nil {
		m.DiskUsage = disk
	}

	return m, nil
}

// getCPUUsage reads /proc/stat and calculates CPU usage percentage
func getCPUUsage() (float64, error) {
	file, err := os.Open("/proc/stat")
	if err != nil {
		return 0, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "cpu ") {
			fields := strings.Fields(line)
			if len(fields) < 8 {
				continue
			}

			var total, idle uint64
			for i := 1; i < len(fields); i++ {
				val, _ := strconv.ParseUint(fields[i], 10, 64)
				total += val
				if i == 4 { // idle is 4th field (index 4)
					idle = val
				}
			}

			// Calculate delta
			if prevCPUTotal > 0 {
				deltaTotal := total - prevCPUTotal
				deltaIdle := idle - prevCPUIdle

				if deltaTotal > 0 {
					usage := float64(deltaTotal-deltaIdle) / float64(deltaTotal) * 100
					prevCPUTotal = total
					prevCPUIdle = idle
					return usage, nil
				}
			}

			prevCPUTotal = total
			prevCPUIdle = idle
			return 0, nil // First call, return 0
		}
	}

	return 0, nil
}

// getRAMUsage reads /proc/meminfo and calculates RAM usage percentage
func getRAMUsage() (float64, error) {
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, err
	}
	defer file.Close()

	var total, available uint64
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		val, _ := strconv.ParseUint(fields[1], 10, 64)
		switch fields[0] {
		case "MemTotal:":
			total = val
		case "MemAvailable:":
			available = val
		}
	}

	if total > 0 {
		used := total - available
		return float64(used) / float64(total) * 100, nil
	}

	return 0, nil
}

// getDiskUsage uses syscall.Statfs to get disk usage
func getDiskUsage(path string) (float64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, err
	}

	total := stat.Blocks * uint64(stat.Bsize)
	free := stat.Bfree * uint64(stat.Bsize)
	used := total - free

	if total > 0 {
		return float64(used) / float64(total) * 100, nil
	}

	return 0, nil
}

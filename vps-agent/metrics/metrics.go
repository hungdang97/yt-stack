package metrics

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// ServiceInfo holds version and status for a single service
type ServiceInfo struct {
	Version string `json:"version"`
	Status  string `json:"status"` // "ok", "down", "unknown"
}

// SystemMetrics holds system resource usage data
type SystemMetrics struct {
	AgentVersion string                 `json:"agent_version"`
	CPUUsage     float64                `json:"cpu_usage"`  // 0-100%
	RAMUsage     float64                `json:"ram_usage"`  // 0-100%
	DiskUsage    float64                `json:"disk_usage"` // 0-100%
	Uptime       int64                  `json:"uptime"`     // seconds
	LastBuild    int64                  `json:"last_build"` // Unix timestamp of last success build
	Services     map[string]ServiceInfo `json:"services"`   // service name → version + status
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
		AgentVersion: "2.0.0",
		Uptime:       int64(time.Since(startTime).Seconds()),
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

	// Collect service versions (parallel, 2s timeout per call, all localhost)
	m.Services = collectServiceVersions()

	return m, nil
}

// services to check health via HTTP /health endpoint
var serviceEndpoints = map[string]string{
	"yt-downloader":    "http://localhost:5001/health",
	"yt-extractor":     "http://localhost:8300/health",
	"tik-downloader":   "http://localhost:5002/health",
	"tik-extractor":    "http://localhost:5555/health",
	"insta-downloader": "http://localhost:5003/health",
	"insta-extractor":  "http://localhost:8000/health",
	"fb-downloader":    "http://localhost:5004/health",
	"fb-extractor":     "http://localhost:8002/health",
	"tw-downloader":    "http://localhost:5005/health",
	"tw-extractor":     "http://localhost:8003/health",
	"uni-downloader":   "http://localhost:5006/health",
	"uni-extractor":    "http://localhost:8004/health",
	"edge-tts":         "http://localhost:8500/health",
	"video-render":     "http://localhost:8501/health",
}

// services to check health via TCP port (no /health endpoint)
// format: name → "addr|docker_container|version_cmd"
var tcpServices = map[string]tcpServiceConfig{
	"nginx": {Addr: "localhost:80"},
	"gost":  {Addr: "localhost:1111"},
}

type tcpServiceConfig struct {
	Addr string
}

var healthClient = &http.Client{Timeout: 2 * time.Second}

// CollectServiceHealth checks health of all services (exported for use by control API)
func CollectServiceHealth() map[string]ServiceInfo {
	return collectServiceVersions()
}

func collectServiceVersions() map[string]ServiceInfo {
	results := make(map[string]ServiceInfo)
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Check HTTP /health services
	for name, url := range serviceEndpoints {
		wg.Add(1)
		go func(name, url string) {
			defer wg.Done()
			info := checkServiceHealth(name, url)
			mu.Lock()
			results[name] = info
			mu.Unlock()
		}(name, url)
	}

	// Check TCP port services (nginx, gost) + get version from docker
	for name, cfg := range tcpServices {
		wg.Add(1)
		go func(name string, cfg tcpServiceConfig) {
			defer wg.Done()
			info := checkTCPHealth(cfg)
			mu.Lock()
			results[name] = info
			mu.Unlock()
		}(name, cfg)
	}

	wg.Wait()
	return results
}

func checkTCPHealth(cfg tcpServiceConfig) ServiceInfo {
	conn, err := net.DialTimeout("tcp", cfg.Addr, 2*time.Second)
	if err != nil {
		return ServiceInfo{Status: "down", Version: "2.0.0"}
	}
	conn.Close()

	return ServiceInfo{Status: "ok", Version: "2.0.0"}
}


func checkServiceHealth(name, url string) ServiceInfo {
	resp, err := healthClient.Get(url)
	if err != nil {
		return ServiceInfo{Status: "down"}
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return ServiceInfo{Status: fmt.Sprintf("error:%d", resp.StatusCode)}
	}

	var data map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return ServiceInfo{Status: "ok"}
	}

	version := ""
	if v, ok := data["version"].(string); ok {
		version = v
	}

	return ServiceInfo{
		Version: version,
		Status:  "ok",
	}
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

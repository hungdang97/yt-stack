package metrics

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// composeFile mirrors the minimal subset of docker-compose.yml we need
// to discover services + their host-side ports.
type composeFile struct {
	Services map[string]composeService `yaml:"services"`
}

type composeService struct {
	Ports []string `yaml:"ports"`
}

// DiscoverServicesFromCompose parses docker-compose.yml and returns:
//   - httpEndpoints: service name → http://localhost:PORT/health  (most services)
//   - tcpEndpoints:  service name → tcp host:port                 (nginx, gost)
//   - allServices:   list of all service names (for restart whitelist)
//
// Convention:
//   - warp-* → infrastructure, skipped (no health monitoring needed)
//   - nginx, gost → TCP port check (no /health endpoint)
//   - everything else with `ports:` → assume HTTP /health on first published port
//
// Service with no `ports:` (e.g. internal-only containers) is skipped.
// Adding/removing a service in docker-compose.yml is sufficient — no agent code change.
func DiscoverServicesFromCompose(composePath string) (
	httpEndpoints map[string]string,
	tcpEndpoints map[string]tcpServiceConfig,
	allServices []string,
	err error,
) {
	httpEndpoints = map[string]string{}
	tcpEndpoints = map[string]tcpServiceConfig{}

	data, err := os.ReadFile(composePath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("read compose: %w", err)
	}
	var cf composeFile
	if err := yaml.Unmarshal(data, &cf); err != nil {
		return nil, nil, nil, fmt.Errorf("parse compose: %w", err)
	}

	for name, svc := range cf.Services {
		if strings.HasPrefix(name, "warp-") {
			continue // infrastructure, no separate health check
		}
		if len(svc.Ports) == 0 {
			continue // internal-only service
		}
		port := extractFirstPort(svc.Ports[0])
		if port == 0 {
			continue
		}

		switch name {
		case "nginx", "gost":
			tcpEndpoints[name] = tcpServiceConfig{
				Addr: fmt.Sprintf("localhost:%d", port),
			}
		default:
			httpEndpoints[name] = fmt.Sprintf("http://localhost:%d/health", port)
		}
		allServices = append(allServices, name)
	}
	return httpEndpoints, tcpEndpoints, allServices, nil
}

// extractFirstPort parses docker-compose port spec.
// Formats handled: "5001:5001", "5001:5001/tcp", "5001", "127.0.0.1:5001:5001".
// Returns the HOST-side port (left of last colon) for localhost connections.
func extractFirstPort(spec string) int {
	s := strings.TrimSpace(spec)
	s = strings.SplitN(s, "/", 2)[0] // strip /tcp /udp suffix

	parts := strings.Split(s, ":")
	// "8505" → ["8505"]            (length 1, single port)
	// "8505:8505" → ["8505","8505"] (length 2, host:container)
	// "127.0.0.1:8505:8505" → ["127.0.0.1","8505","8505"] (length 3, ip:host:container)
	if len(parts) == 0 {
		return 0
	}
	var hostPort string
	if len(parts) == 1 {
		hostPort = parts[0]
	} else {
		// host port is second-from-last
		hostPort = parts[len(parts)-2]
	}
	p, err := strconv.Atoi(hostPort)
	if err != nil {
		return 0
	}
	return p
}

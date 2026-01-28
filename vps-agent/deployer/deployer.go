package deployer

import (
	"encoding/json"
	"log"
	"os/exec"
)

type Deployer struct {
	projectDir string
}

func NewDeployer(projectDir string) *Deployer {
	return &Deployer{projectDir: projectDir}
}

func (d *Deployer) PullCode() error {
	log.Println("[Deployer] Pulling latest code from git...")

	// Reset any local changes to ensure pull works
	exec.Command("git", "-C", d.projectDir, "reset", "--hard").Run()

	cmd := exec.Command("git", "pull")
	cmd.Dir = d.projectDir
	output, err := cmd.CombinedOutput()

	if err != nil {
		log.Printf("[Deployer] Git pull failed: %s", output)
		return err
	}

	log.Printf("[Deployer] Git pull output: %s", output)
	return nil
}

func (d *Deployer) Build() error {
	log.Println("[Deployer] Building service (Clean Build)...")

	// Use --no-cache to ensure we don't use old layers
	cmd := exec.Command("docker-compose", "build", "--no-cache")
	cmd.Dir = d.projectDir
	output, err := cmd.CombinedOutput()

	if err != nil {
		log.Printf("[Deployer] Build failed: %s", output)
		return err
	}

	log.Println("[Deployer] Build successful")
	return nil
}

func (d *Deployer) Deploy() error {
	log.Println("[Deployer] Deploying service...")

	// Add --remove-orphans and --force-recreate to ensure structure updates apply
	cmd := exec.Command("docker-compose", "up", "-d", "--remove-orphans", "--force-recreate")
	cmd.Dir = d.projectDir
	output, err := cmd.CombinedOutput()

	if err != nil {
		log.Printf("[Deployer] Deploy failed: %s", output)
		return err
	}

	log.Println("[Deployer] Deploy successful")
	return nil
}

func (d *Deployer) Stop() error {
	// Down with --rmi local to remove images built by check, ensuring next build is fresh
	cmd := exec.Command("docker-compose", "down", "--remove-orphans")
	cmd.Dir = d.projectDir
	return cmd.Run()
}

func (d *Deployer) GetStatus() (map[string]interface{}, error) {
	cmd := exec.Command("docker-compose", "ps", "--format", "json")
	cmd.Dir = d.projectDir
	output, err := cmd.Output()

	if err != nil {
		return nil, err
	}

	var status []map[string]interface{}
	json.Unmarshal(output, &status)

	return map[string]interface{}{
		"containers": status,
		"healthy":    len(status) > 0,
	}, nil
}

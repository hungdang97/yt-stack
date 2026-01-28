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

func (d *Deployer) Build() error {
	log.Println("[Deployer] Building service...")

	cmd := exec.Command("docker-compose", "build")
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

	cmd := exec.Command("docker-compose", "up", "-d")
	cmd.Dir = d.projectDir
	output, err := cmd.CombinedOutput()

	if err != nil {
		log.Printf("[Deployer] Deploy failed: %s", output)
		return err
	}

	log.Println("[Deployer] Deploy successful")
	return nil
}

func (d *Deployer) Restart() error {
	log.Println("[Deployer] Restarting service...")

	cmd := exec.Command("docker-compose", "restart")
	cmd.Dir = d.projectDir
	output, err := cmd.CombinedOutput()

	if err != nil {
		log.Printf("[Deployer] Restart failed: %s", output)
		return err
	}

	log.Println("[Deployer] Restart successful")
	return nil
}

func (d *Deployer) Stop() error {
	cmd := exec.Command("docker-compose", "down")
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

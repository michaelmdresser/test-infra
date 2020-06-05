package deployer

import (
	"fmt"
	"os"
	"path/filepath"

	"k8s.io/test-infra/kubetest2/pkg/exec"
)

// TODO: test this?
func getVarSetByConfig(kubernetesRepoPath, varName string) (string, error) {
	cmd := exec.Command(
		"/bin/bash",
		"-c",
		fmt.Sprintf("source %s; echo $%s", filepath.Join(kubernetesRepoPath, "cluster", "gce", "config-test.sh"), varName),
	)
	lines, err := exec.OutputLines(cmd)
	if err != nil {
		return "", fmt.Errorf("couldn't get env var %s: %s", varName, err)
	}
	if len(lines) == 0 {
		return "", fmt.Errorf("getting env var %s had no output", varName)
	}
	if len(lines) > 1 {
		return "", fmt.Errorf("getting env var %s had more than one line of output", varName)
	}

	return lines[0], nil
}

// TODO: idempotent
func deleteKubeconfig() error {
	err := os.Remove(kubeconfigPath)
	// TODO: should is not exist print a log at least?
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("Could not remove kubeconfig: %s", err)
	}
	return nil
}

// Calculates the cluster IP range based on the no. of nodes in the cluster.
// Note: This mimics the function get-cluster-ip-range used by kube-up script.
// Copied from kubetest's bash.go
func getClusterIPRange(numNodes int) string {
	suggestedRange := "10.64.0.0/14"
	if numNodes > 1000 {
		suggestedRange = "10.64.0.0/13"
	}
	if numNodes > 2000 {
		suggestedRange = "10.64.0.0/12"
	}
	if numNodes > 4000 {
		suggestedRange = "10.64.0.0/11"
	}
	return suggestedRange
}

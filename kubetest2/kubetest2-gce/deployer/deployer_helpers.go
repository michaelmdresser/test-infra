package deployer

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"k8s.io/test-infra/kubetest2/pkg/exec"
)

func (d *deployer) setKubernetesPathIfNotSet() error {
	// TODO: also verify that the set path is a kube repo
	if d.kubernetesRootPath == "" {
		// TODO: current dir vs gopaths like build.go?
		path, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("Failed to get current working directory for setting Kubernetes root path: %s", err)
		}
		d.kubernetesRootPath = path
	}

	return nil
}

// verifyBuildFlags only checks flags that are needed for Build
func (d *deployer) verifyBuildFlags() error {
	if err := d.setKubernetesPathIfNotSet(); err != nil {
		return err
	}
	return nil
}

func (d *deployer) verifyFlags() error {
	if !d.multizone {
		// see util.sh (GCE) test-setup, test-teardown
		if os.Getenv("MULTIZONE") == "true" {
			log.Print("multizone flag = true retrieved from environment $MULTIZONE. This is a deprecated functionality.")
			d.multizone = true
		}
	}
	if len(d.e2eZones) == 0 {
		// see util.sh (GCE) test-setup, test-teardown
		if zonesString := os.Getenv("E2E_ZONES"); len(zonesString) > 0 {
			parsed := strings.Split(zonesString, " ")
			log.Printf("E2E zones will be set to %+v based on environment $E2E_ZONES. This is a deprecated functionality.", parsed)
			d.e2eZones = parsed
		}
	}
	if d.numNodes < 1 {
		log.Print("Num nodes was not set. Defaulting to NUM_NODES environment variable. This functionality is deprecated.")
		if numNodes, err := strconv.Atoi(os.Getenv("NUM_NODES")); err == nil {
			d.numNodes = numNodes
		} else {
			return fmt.Errorf("Failed to get NUM_NODES from environment: %s", err)
		}
	}
	if d.clusterIPRange == "" {
		d.clusterIPRange = getClusterIPRange(d.numNodes)
	}
	// TODO: set a flag to acquire a GCP project from boskos
	if d.gcpProject == "" {
		return fmt.Errorf("no GCP project was set")
	}
	if d.gcpZone == "" {
		return fmt.Errorf("no GCP zone was set")
	}
	if err := d.setKubernetesPathIfNotSet(); err != nil {
		return err
	}
	if d.multizone && len(d.e2eZones) == 0 {
		return fmt.Errorf("multizone set but no E2E zones provided")
	}

	// we need to extract the NODE_TAG and NETWORK that config-test.sh
	// sets in order to properly create/delete firewall rules that will
	// match what kube-up is going to do
	// TODO: this is incredibly messy
	nodeTag, err := getVarSetByConfig(d.kubernetesRootPath, "NODE_TAG")
	if err != nil {
		return err
	}
	d.nodeTag = nodeTag

	network, err := getVarSetByConfig(d.kubernetesRootPath, "NETWORK")
	if err != nil {
		return err
	}
	d.network = network

	return nil
}

func (d *deployer) buildCommonEnv() []string {
	env := os.Environ()

	env = append(env, fmt.Sprintf("PROJECT=%s", d.gcpProject))
	env = append(env, fmt.Sprintf("ZONE=%s", d.gcpZone))
	env = append(env, fmt.Sprintf("KUBECONFIG=%s", kubeconfigPath))
	env = append(env, fmt.Sprintf("KUBE_ROOT=%s", d.kubernetesRootPath))
	env = append(env, fmt.Sprintf("CLUSTER_IP_RANGE=%s", d.clusterIPRange))

	// don't want to depend on Go's choice of string representation
	// for booleans
	var nodeLoggingEnv string
	if d.nodeLoggingEnabled {
		nodeLoggingEnv = "true"
	} else {
		nodeLoggingEnv = "false"
	}
	env = append(env, fmt.Sprintf("KUBE_ENABLE_NODE_LOGGING=%s", nodeLoggingEnv))

	// from e2e-up.sh (or status or down)
	env = append(env, fmt.Sprintf("KUBECTL=%s", filepath.Join(d.kubernetesRootPath, "cluster", "kubectl.sh")))
	env = append(env, fmt.Sprintf("KUBE_CONFIG_FILE=%s", "config-test.sh"))

	// TODO: this is set by cluster/kube-util.sh, not sure if necessary
	// probably not, because kube up calls kube util
	env = append(env, fmt.Sprintf("KUBERNETES_PROVIDER=gce"))

	return env
}

func (d *deployer) kubeconfigIsPresent() error {
	_, err := os.Stat(kubeconfigPath)
	return err

	// TODO: this block necessary?
	/*
		if err == nil {
			return nil
		} else if os.IsNotExist(err) {
			return err
		}
		return fmt.Errorf("Kubeconfig may or may not exist. Err: %s")
	*/
}

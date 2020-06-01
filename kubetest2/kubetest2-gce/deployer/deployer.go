/*
Copyright 2020 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package deployer implements the kubetest2 GKE deployer
package deployer

import (
	"fmt"
	// "io/ioutil"
	"log"
	"os"
	// realexec "os/exec" // Only for ExitError; Use kubetest2/pkg/exec to actually exec stuff
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/pflag"

	// "k8s.io/test-infra/kubetest2/pkg/build"
	"k8s.io/test-infra/kubetest2/pkg/exec"
	"k8s.io/test-infra/kubetest2/pkg/metadata"
	"k8s.io/test-infra/kubetest2/pkg/types"
)

// Name is the name of the deployer
const Name = "gce"

var kubeconfigPath = filepath.Join(os.TempDir(), "kubetest2-kubeconfig")

type deployer struct {
	// generic parts
	commonOptions types.Options

	localLogsDir string

	clusterIPRange          string
	gcpProject              string
	gcpZone                 string
	gcpSSHProxyInstanceName string
	nodeLoggingEnabled      bool
	// path to the root of the local kubernetes repo
	kubernetesRootPath string

	// TODO: is multizone necessary as a cmd line flag if e2e zones is set?
	multizone bool
	e2eZones  []string

	numNodes int

	// TODO: necessary?
	testPrepared bool
}

// New implements deployer.New for gke
func New(opts types.Options) (types.Deployer, *pflag.FlagSet) {
	// create a deployer object and set fields that are not flag controlled
	d := &deployer{
		commonOptions: opts,
		localLogsDir:  filepath.Join(opts.ArtifactsDir(), "logs"),
	}

	// register flags and return
	return d, bindFlags(d)
}

// verifyFlags validates that required flags are set, as well as
// ensuring that location() will not return errors.
/*
func (d *deployer) verifyFlags() error {
	if d.cluster == "" {
		return fmt.Errorf("--cluster-name must be set for GKE deployment")
	}
	if d.project == "" {
		return fmt.Errorf("--project must be set for GKE deployment")
	}
	if _, err := d.location(); err != nil {
		return err
	}
	return nil
}

func (d *deployer) location() (string, error) {
	if d.zone == "" && d.region == "" {
		return "", fmt.Errorf("--zone or --region must be set for GKE deployment")
	} else if d.zone != "" && d.region != "" {
		return "", fmt.Errorf("--zone and --region cannot both be set")
	}

	if d.zone != "" {
		return "--zone=" + d.zone, nil
	}
	return "--region=" + d.region, nil
}
*/

// assert that New implements types.NewDeployer
var _ types.NewDeployer = New

func bindFlags(d *deployer) *pflag.FlagSet {
	flags := pflag.NewFlagSet(Name, pflag.ContinueOnError)

	flags.StringVar(&d.clusterIPRange, "cluster-ip-range", "", "Cluster IP Range. Defaults based on number of nodes if not set.")
	flags.StringVar(&d.gcpProject, "gcp-project", "", "GCP Project to create VMs in. Must be set.")
	flags.StringVar(&d.gcpZone, "gcp-zone", "", "GCP Zone to create VMs in. Must be set.")
	flags.StringVar(&d.gcpSSHProxyInstanceName, "gcp-ssh-proxy-instance-name", "", "Name of instance for SSH Proxy.")
	flags.BoolVar(&d.nodeLoggingEnabled, "node-logging-enabled", false, "If node logging should be enabled.")
	flags.StringVar(&d.kubernetesRootPath, "kubernetes-root-path", "", "The path to the local Kubernetes repo. Necessary to call certain scripts. Defaults to the current directory.")
	flags.BoolVar(&d.multizone, "multizone", false, "Whether testing should be multizone.")
	flags.StringSliceVar(&d.e2eZones, "e2e-zones", []string{}, "Zones for multizone testing. Only considered if multizone is set.")
	flags.IntVar(&d.numNodes, "num-nodes", 0, "Number of nodes to run the cluster on. Defaults to the environment variable NUM_NODES if not set (or set to < 1).")
	return flags
}

func (d *deployer) verifyFlags() error {
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
	// TODO: default based on gcloud?
	if d.gcpProject == "" {
		return fmt.Errorf("no GCP project was set")
	}
	if d.gcpZone == "" {
		return fmt.Errorf("no GCP zone was set")
	}
	// TODO: also verify that the set path is a kube repo
	if d.kubernetesRootPath == "" {
		path, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("Failed to get current working directory for Kubernetes root path: %s", err)
		}
		d.kubernetesRootPath = path
	}
	if d.multizone && len(d.e2eZones) == 0 {
		return fmt.Errorf("multizone set but no e2e zones provided")
	}
	return nil
}

// assert that deployer implements types.Deployer
var _ types.Deployer = &deployer{}

func (d *deployer) Provider() string {
	return "gce"
}

func (d *deployer) Build() error {
	// TODO: handle build
	/*
		if err := build.Build(); err != nil {
			return err
		}

		if d.stageLocation != "" {
			if err := build.Stage(d.stageLocation); err != nil {
				return fmt.Errorf("error staging build: %v", err)
			}
		}
	*/
	return nil
}

func kubeconfigIsPresent() error {
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

// TODO: idempotent
func deleteKubeconfig() error {
	err := os.Remove(kubeconfigPath)
	if err != nil {
		return fmt.Errorf("Could not remove kubeconfig: %s")
	}
	return nil
}

func (d *deployer) buildCommonEnv() []string {
	env := os.Environ()

	env = append(env, fmt.Sprintf("PROJECT=%s", d.gcpProject))
	env = append(env, fmt.Sprintf("ZONE=%s", d.gcpZone))
	env = append(env, fmt.Sprintf("KUBECONFIG=%s", kubeconfigPath))
	env = append(env, fmt.Sprintf("KUBE_ROOT=%s", d.kubernetesRootPath))
	env = append(env, fmt.Sprintf("CLUSTER_IP_RANGE=%s", d.clusterIPRange))
	env = append(env, fmt.Sprintf("KUBE_ENABLE_NODE_LOGGING=%s", d.nodeLoggingEnabled))

	return env
}

func (d *deployer) createFirewallRules() error {
	cmd := exec.Command(
		"gcloud", "compute", "firewall-rules", "create",
		"--project="+d.gcpProject,
		// TODO: figure out proper value for node tag
		// "--target-tags=NODE_TAG"
		"--allow tcp:80,tcp:8080",
		// TODO: figur eout proper value, should be default or set by env var
		// "--notework NETWORK"
		// TODO: update after geetting correct node tag value
		"NODE_TAG-http-alt",
	)
	exec.NoOutput(cmd)
	err := cmd.Run()
	if err != nil {
		// TODO: handle error (bundle up?)
	}

	cmd = exec.Command(
		"gcloud", "compute", "firewall-rules", "create",
		"--project "+d.gcpProject,
		// TODO: set properly
		// "--target-tags NODE_TAG",
		"--allow tcp:30000-32767,udp:30000-32767",
		// TODO
		// "--network NETWORK",
		"NODE_TAG-nodeports",
	)
	exec.NoOutput(cmd)
	err = cmd.Run()
	if err != nil {
		// TODO handle error
	}

	return nil
}

func (d *deployer) ensureFirewallRules() error {
	// TODO
	return nil
}

func (d *deployer) deleteFirewallRules() error {
	cmd := exec.Command(
		"gcloud", "compute", "firewall-rules", "delete",
		"--project "+d.gcpProject,
		// TODO: verify and move to const like create
		"NODE_TAG-http-alt",
	)
	exec.NoOutput(cmd)
	err := cmd.Run()
	if err != nil {
		// TODO: handle (bundle)
	}

	cmd = exec.Command(
		"gcloud", "compute", "firewall-rules", "delete",
		"--project "+d.gcpProject,
		// TODO: verify and move to const like create
		"NODE_TAG-nodeports",
	)
	exec.NoOutput(cmd)
	err = cmd.Run()
	if err != nil {
		// TODO: handle (bundle)
	}

	return nil
}

func (d *deployer) Up() error {
	if err := d.verifyFlags(); err != nil {
		return err
	}
	if err := kubeconfigIsPresent(); err != nil {
		return fmt.Errorf("Kubeconfig is not present at expected path: %s", err)
	}

	kubeUpEnv := d.buildCommonEnv()

	script := filepath.Join(d.kubernetesRootPath, "cluster", "kube-up.sh")
	if d.multizone {
		multiErrs := make([]error, len(d.e2eZones))
		for i, zone := range d.e2eZones {
			multiEnv := kubeUpEnv
			if i != 0 {
				multiEnv = append(multiEnv, "KUBE_USE_EXISTING_MASTER=true")
			} else {
				multiEnv = append(multiEnv, "KUBE_USE_EXISTING_MASTER=false")
			}
			multiEnv = append(multiEnv, "KUBE_GCE_ZONE=%s", zone)

			cmd := exec.Command(script)
			exec.NoOutput(cmd)
			// TODO: does this actually work?
			cmd.SetEnv(multiEnv...)
			err := cmd.Run()
			if err != nil {
				multiErrs = append(multiErrs, fmt.Errorf(zone+": %s", err))
			}
		}
		if len(multiErrs) > 0 {
			return fmt.Errorf("Error(s) encountered during multizone up:\n%+v", multiErrs)
		}
	} else {
		cmd := exec.Command(script)
		exec.NoOutput(cmd)
		// TODO: does this work?
		cmd.SetEnv(kubeUpEnv...)
		err := cmd.Run()
		if err != nil {
			return fmt.Errorf("Error encountered during kube-up: %s", err)
		}
	}

	if err := d.createFirewallRules(); err != nil {
		return err
	}

	if err := d.ensureFirewallRules(); err != nil {
		return err
	}

	return nil
}

// TODO: verify if this is how we want to handle this
func (d *deployer) IsUp() (up bool, err error) {
	// naively assume that if the api server reports nodes, the cluster is up
	lines, err := exec.CombinedOutputLines(
		// TODO: set env properly (the kubeconfig location)
		exec.Command("kubectl", "get", "nodes", "-o=name"),
	)
	if err != nil {
		return false, metadata.NewJUnitError(err, strings.Join(lines, "\n"))
	}
	return len(lines) > 0, nil
}

func (d *deployer) DumpClusterLogs() error {
	localArtifactsDir := d.localLogsDir
	// TODO: enable gcs dump?
	logexporterGCSPath := ""

	// mostly copied from e2e.go
	logDumpPath := filepath.Join(d.kubernetesRootPath, "cluster", "log-dump", "log-dump.sh")
	// TODO: do we need to keep this code path?
	// cluster/log-dump/log-dump.sh only exists in the Kubernetes tree
	// post-1.3. If it doesn't exist, print a debug log but do not report an error.
	if _, err := os.Stat(logDumpPath); err != nil {
		log.Printf("Could not find %s. This is expected if running tests against a Kubernetes 1.3 or older tree.", logDumpPath)
		if cwd, err := os.Getwd(); err == nil {
			log.Printf("CWD: %v", cwd)
		}
		return nil
	}
	// TODO: set env for running log dump
	if logexporterGCSPath != "" {
		log.Printf("Dumping logs from nodes to GCS directly at path: %v", logexporterGCSPath)
		cmd := exec.Command(logDumpPath, localArtifactsDir, logexporterGCSPath)
		exec.NoOutput(cmd)
		return cmd.Run()
	}

	log.Printf("Dumping logs locally to: %v", localArtifactsDir)
	cmd := exec.Command(logDumpPath, localArtifactsDir)
	exec.NoOutput(cmd)
	return cmd.Run()
}

func (d *deployer) TestSetup() error {
	// TODO: does this matter?
	if d.testPrepared {
		// Ensure setup is a singleton.
		return nil
	}
	if _, err := d.Kubeconfig(); err != nil {
		return err
	}
	// TODO: i think this must be implemented
	if err := d.ensureFirewallRules(); err != nil {
		return err
	}
	d.testPrepared = true
	return nil
}

// Kubeconfig returns a path to a kubeconfig file for the cluster in
// a temp directory, creating one if one does not exist.
// It also sets the KUBECONFIG environment variable appropriately.
func (d *deployer) Kubeconfig() (string, error) {
	if err := kubeconfigIsPresent(); err != nil {
		return "", err
	}
	return kubeconfigPath, nil
}

func (d *deployer) Down() error {
	if err := d.verifyFlags(); err != nil {
		return err
	}
	if err := kubeconfigIsPresent(); err != nil {
		return fmt.Errorf("Kubeconfig is not present at expected path: %s", err)
	}

	if err := d.deleteFirewallRules(); err != nil {
		return err
	}

	kubeDownEnv := d.buildCommonEnv()

	script := filepath.Join(d.kubernetesRootPath, "cluster", "kube-down.sh")
	if d.multizone {
		multiErrs := make([]error, len(d.e2eZones))

		// iterate in reverse order like test-teardown
		for i := len(d.e2eZones) - 1; i >= 0; i-- {
			zone := d.e2eZones[i]

			multiEnv := kubeDownEnv
			// on the last one, use existing should be false
			if i == (len(d.e2eZones) - 1) {
				multiEnv = append(multiEnv, "KUBE_USE_EXISTING_MASTER=false")
			} else {
				multiEnv = append(multiEnv, "KUBE_USE_EXISTING_MASTER=true")
			}
			multiEnv = append(multiEnv, "KUBE_GCE_ZONE=%s", zone)

			cmd := exec.Command(script)
			exec.NoOutput(cmd)
			// TODO: does this work?
			cmd.SetEnv(multiEnv...)
			err := cmd.Run()
			if err != nil {
				multiErrs = append(multiErrs, fmt.Errorf(zone+": %s", err))
			}
		}
		if len(multiErrs) > 0 {
			return fmt.Errorf("Error(s) encountered during multizone down:\n%+v", multiErrs)
		}
	} else {
		cmd := exec.Command(script)
		exec.NoOutput(cmd)
		// TODO: does this work?
		cmd.SetEnv(kubeDownEnv...)
		err := cmd.Run()
		if err != nil {
			return fmt.Errorf("Error encountered during kube-down: %s", err)
		}
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

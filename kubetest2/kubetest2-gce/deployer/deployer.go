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
	// TODO: clean up imports
	// "io/ioutil"
	// realexec "os/exec" // Only for ExitError; Use kubetest2/pkg/exec to actually exec stuff
	"fmt"
	"log"
	"os"
	"path/filepath"
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
	// TODO: pull artifacts from GCS -- propagate to the flag verifiers

	multizone bool
	e2eZones  []string

	numNodes int

	// TODO: necessary?
	testPrepared bool

	// these are needed to create firewall rules, however their
	// values are determined by config-test.sh. We cannot include
	// these as a flag to the program at the moment, because they
	// will be overridden when cluster/gce/util.sh sources
	// config-test.sh when we call kube-up.sh or kube-down.sh.
	// They are going to be set by sourcing config-test.sh in this
	// program and finding the values of NODE_TAG and NETWORK.
	nodeTag string
	network string
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
	flags.BoolVar(&d.multizone, "multizone", false, "Whether testing should be multizone. If false (default), checks $MULTIZONE (deprecated) in case of legacy override.")
	flags.StringSliceVar(&d.e2eZones, "e2e-zones", []string{}, "Zones for multizone testing. Only considered if multizone is set. If not set, checks $E2E_ZONES (deprecated) before defaulting to empty slice.")
	flags.IntVar(&d.numNodes, "num-nodes", 0, "Number of nodes to run the cluster on. Defaults to the environment variable NUM_NODES if not set (or set to < 1).")
	return flags
}

// assert that deployer implements types.Deployer
var _ types.Deployer = &deployer{}

func (d *deployer) Provider() string {
	return "gce"
}

func (d *deployer) Build() error {
	// TODO: handle GCE pull

	if err := d.verifyBuildFlags(); err != nil {
		return err
	}

	cmd := exec.Command("make", "-C", d.kubernetesRootPath, "bazel-release")
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("error during make step of build: %s", err)
	}

	// no untarring, uploading, etc is necessary because
	// kube-up/down use find-release-tars and upload-tars
	// which know how to find the tars, assuming KUBE_ROOT
	// is set

	return nil
}

func (d *deployer) Up() error {
	if err := d.verifyFlags(); err != nil {
		return err
	}
	if err := d.kubeconfigIsPresent(); err != nil {
		return fmt.Errorf("Kubeconfig is not present at expected path: %s", err)
	}

	kubeUpEnv := d.buildCommonEnv()

	script := filepath.Join(d.kubernetesRootPath, "cluster", "kube-up.sh")
	if d.multizone {
		// in a multizone deployment, we have to iterate over each zone and call kube-down for each
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
	// a warning: log-dump.sh does not exist in Kubernetes 1.3 or earlier. This should not be a concern.
	logDumpPath := filepath.Join(d.kubernetesRootPath, "cluster", "log-dump", "log-dump.sh")
	if _, err := os.Stat(logDumpPath); err != nil {
		return fmt.Errorf("calling stat for log-dump.sh at %s failed: %s", logDumpPath, err)
	}

	// TODO: check env for multizone stuff
	dumpEnv := d.buildCommonEnv()
	if logexporterGCSPath != "" {
		log.Printf("Dumping logs from nodes to GCS directly at path: %v", logexporterGCSPath)
		cmd := exec.Command(logDumpPath, localArtifactsDir, logexporterGCSPath)
		exec.NoOutput(cmd)
		cmd.SetEnv(dumpEnv...)
		return cmd.Run()
	}

	log.Printf("Dumping logs locally to: %v", localArtifactsDir)
	cmd := exec.Command(logDumpPath, localArtifactsDir)
	exec.NoOutput(cmd)
	cmd.SetEnv(dumpEnv...)
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
	if err := d.kubeconfigIsPresent(); err != nil {
		return "", err
	}
	return kubeconfigPath, nil
}

func (d *deployer) Down() error {
	if err := d.verifyFlags(); err != nil {
		return err
	}
	if err := d.kubeconfigIsPresent(); err != nil {
		return fmt.Errorf("Kubeconfig is not present at expected path: %s", err)
	}

	if err := d.deleteFirewallRules(); err != nil {
		return err
	}

	kubeDownEnv := d.buildCommonEnv()

	script := filepath.Join(d.kubernetesRootPath, "cluster", "kube-down.sh")
	if d.multizone {
		// in a multizone deployment, we have to iterate over each zone and call kube-down for each

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
		cmd.SetEnv(kubeDownEnv...)
		err := cmd.Run()
		if err != nil {
			return fmt.Errorf("Error encountered during kube-down: %s", err)
		}
	}

	return nil
}

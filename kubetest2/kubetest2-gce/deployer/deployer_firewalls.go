package deployer

import (
	"fmt"

	"k8s.io/test-infra/kubetest2/pkg/exec"
)

func (d *deployer) createFirewallRules() error {
	errs := []error{}

	cmd := exec.Command(
		"gcloud", "compute", "firewall-rules", "create",
		"--project="+d.gcpProject,
		"--target-tags="+d.nodeTag,
		"--allow tcp:80,tcp:8080",
		"--network "+d.network,
		d.nodeTag+"-http-alt",
	)
	exec.NoOutput(cmd)
	err := cmd.Run()
	if err != nil {
		errs = append(errs, err)
	}

	cmd = exec.Command(
		"gcloud", "compute", "firewall-rules", "create",
		"--project "+d.gcpProject,
		"--target-tags "+d.nodeTag,
		"--allow tcp:30000-32767,udp:30000-32767",
		"--network "+d.network,
		d.nodeTag+"-nodeports",
	)
	exec.NoOutput(cmd)
	err = cmd.Run()
	if err != nil {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return fmt.Errorf("at least one error occurred during firewall creation: %+v", errs)
	}
	return nil
}

func (d *deployer) ensureFirewallRules() error {
	// TODO
	return nil
}

func (d *deployer) deleteFirewallRules() error {
	errs := []error{}

	cmd := exec.Command(
		"gcloud", "compute", "firewall-rules", "delete",
		"--project "+d.gcpProject,
		d.nodeTag+"-http-alt",
	)
	exec.NoOutput(cmd)
	err := cmd.Run()
	if err != nil {
		errs = append(errs, err)
	}

	cmd = exec.Command(
		"gcloud", "compute", "firewall-rules", "delete",
		"--project "+d.gcpProject,
		d.nodeTag+"-nodeports",
	)
	exec.NoOutput(cmd)
	err = cmd.Run()
	if err != nil {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return fmt.Errorf("at least one error occurred during firewall creation: %+v", errs)
	}
	return nil
}

package main

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// requireBlankDataDisk is a fail-closed, read-only guard for the quick-setup
// wizard. Standard advanced partition management does not use this guard.
func requireBlankDataDisk(ctx context.Context, device string) error {
	output, err := exec.CommandContext(ctx, "/usr/bin/lsblk", "-nrpo", "NAME,TYPE,PTTYPE,FSTYPE", device).CombinedOutput()
	if err != nil {
		return fmt.Errorf("inspect blank disk layout: %s", strings.TrimSpace(string(output)))
	}
	if err := validateBlankDiskLayout(string(output), device); err != nil {
		return err
	}
	// wipefs -n is inspection only. Reject filesystem, partition-table, RAID
	// and LVM signatures, including stale signatures not shown by lsblk.
	output, err = exec.CommandContext(ctx, "/usr/sbin/wipefs", "--no-act", "--noheadings", "--output", "TYPE,OFFSET", "--", device).CombinedOutput()
	if err != nil {
		return fmt.Errorf("inspect disk signatures: %s", strings.TrimSpace(string(output)))
	}
	if strings.TrimSpace(string(output)) != "" {
		return errors.New("disk contains existing signatures; quick setup is restricted to fully blank disks")
	}
	return nil
}

func validateBlankDiskLayout(raw, device string) error {
	lines := strings.Split(strings.TrimSpace(raw), "\n")
	if len(lines) != 1 {
		return errors.New("disk contains partitions or dependent devices; quick setup refused")
	}
	fields := strings.Fields(lines[0])
	if len(fields) != 2 || fields[0] != device || fields[1] != "disk" {
		return errors.New("disk is not blank or is not an independent disk")
	}
	return nil
}

package main

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/frigate"
)

// The fixed code is executed in the already running, managed official image.
// Never snapshot or roll back a database that the configuration moved outside
// /config: the media archive is intentionally not part of the small snapshot.
func verifyFrigateDatabaseLocation(ctx context.Context, binary, name string) error {
	const script = `import pathlib
from ruamel.yaml import YAML
root = pathlib.Path('/config')
files = [root / 'config.yaml', root / 'config.yml']
source = next((p for p in files if p.is_file()), None)
if source is None:
    raise SystemExit(2)
config = YAML(typ="safe").load(source.read_text()) or {}
value = (config.get('database') or {}).get('path', '/config/frigate.db')
if not isinstance(value, str):
    raise SystemExit(3)
path = pathlib.Path(value)
if not path.is_absolute() or not path.resolve().is_relative_to(root.resolve()):
    raise SystemExit(4)
print('managed-config-database')`
	out, err := exec.CommandContext(ctx, binary, "exec", name, "python3", "-c", script).CombinedOutput()
	if err != nil || strings.TrimSpace(string(out)) != "managed-config-database" {
		return errors.New("Frigate automatic rollback requires the database path to remain inside /config; fix the database path before upgrading")
	}
	return nil
}

// recoverFrigateRegistered reconciles a journal with the immutable image that
// Core has actually committed. No remote image is fetched and no new version
// is installed by this operation. It is safe to expose through the Job Engine.
func recoverFrigateRegistered(ctx context.Context, image string) (moduleContainerStatus, string, error) {
	if !frigate.ValidImage(image) {
		return moduleContainerStatus{}, "", errors.New("invalid registered Frigate image")
	}
	binary, err := exec.LookPath("docker")
	if err != nil {
		return moduleContainerStatus{}, "", err
	}
	txn, err := readFrigateTransaction()
	if err != nil {
		return moduleContainerStatus{}, "", err
	}
	name := moduleContainerName(frigate.ModuleID)
	if txn == nil {
		if _, err := verifyFrigateContainer(ctx, binary, name, image); err != nil {
			return moduleContainerStatus{}, "", err
		}
		status, err := inspectModuleContainer(ctx, binary, name)
		return status, "no pending Frigate upgrade transaction", err
	}
	critical, cancel := context.WithTimeout(context.WithoutCancel(ctx), 9*time.Minute)
	defer cancel()
	if image == txn.NewImage && (txn.Phase == "swapped" || txn.Phase == "committed") {
		status, err := commitFrigateUpgrade(critical, binary, txn)
		return status, "committed Frigate upgrade reconciled; verified backup retained", err
	}
	if image == txn.OldImage && txn.Phase != "committed" {
		status, err := recoverFrigateTransaction(critical, binary, txn)
		return status, "interrupted Frigate upgrade rolled back to the registered image and database", err
	}
	return moduleContainerStatus{}, "", errors.New("Frigate registry does not match the upgrade journal; refusing destructive recovery")
}

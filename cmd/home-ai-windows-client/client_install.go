package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/version"
	"github.com/DeadSoulf/home-ai-core/internal/windowsclient"
)

func runClient(args []string) error {
	if len(args) == 0 {
		return errors.New("client requires install, status, version, check-update or update")
	}
	command := args[0]
	switch command {
	case "install", "status", "version", "check-update", "update", "apply-update":
	case "help", "-h", "--help":
		printUsage()
		return nil
	default:
		return fmt.Errorf("unknown client command %q", command)
	}

	fs := flag.NewFlagSet("client "+command, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var restartAgent bool
	var configPath string
	var candidatePath string
	var updateVersion string
	var updateSHA string
	var waitPID int
	if command == "update" {
		fs.BoolVar(&restartAgent, "restart-agent", false, "restart the Home-AI sync agent after applying the update")
		fs.StringVar(&configPath, "config", "", "sync profile file used when restarting the agent")
	}
	if command == "apply-update" {
		fs.StringVar(&candidatePath, "candidate", "", "downloaded update executable")
		fs.StringVar(&updateVersion, "version", "", "downloaded update version")
		fs.StringVar(&updateSHA, "sha256", "", "verified downloaded update SHA-256")
		fs.IntVar(&waitPID, "wait-pid", 0, "parent process ID to wait for")
		fs.StringVar(&configPath, "restart-agent-config", "", "sync profile file used to restart the agent")
	}
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("client commands do not accept positional arguments")
	}

	switch command {
	case "install":
		executable, err := os.Executable()
		if err != nil {
			return fmt.Errorf("locate Windows client executable: %w", err)
		}
		path, err := windowsclient.InstallUserClient(executable)
		if err != nil {
			return err
		}
		fmt.Printf("Home-AI Windows client installed: %s\n", path)
		fmt.Printf("Version: %s\n", version.Version)
		return nil
	case "status":
		path, installed, err := windowsclient.UserClientInstallStatus()
		if err != nil {
			return err
		}
		fmt.Printf("Running version: %s\n", version.Version)
		if !installed {
			fmt.Printf("Home-AI Windows client is not installed. Expected path: %s\n", path)
			return nil
		}
		fmt.Printf("Home-AI Windows client installed: %s\n", path)
		return nil
	case "version":
		fmt.Println(version.Version)
		return nil
	case "check-update":
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		update, err := windowsclient.DefaultWindowsReleaseSource().FindUpdate(ctx, version.Version)
		if err != nil {
			return updateCheckError(err)
		}
		if update == nil {
			fmt.Printf("Home-AI Windows client %s is up to date.\n", version.Version)
			return nil
		}
		fmt.Printf("Update available: %s -> %s\n", version.Version, update.Version)
		return nil
	case "update":
		restartConfig := ""
		if restartAgent {
			if configPath == "" {
				var err error
				configPath, err = defaultSyncConfigPath()
				if err != nil {
					return err
				}
			}
			configPath, err := filepath.Abs(configPath)
			if err != nil {
				return fmt.Errorf("resolve sync config path: %w", err)
			}
			if err := validateAgentReady(configPath); err != nil {
				return err
			}
			restartConfig = configPath
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		started, nextVersion, err := startClientUpdate(ctx, restartConfig)
		if err != nil {
			return err
		}
		if !started {
			fmt.Printf("Home-AI Windows client %s is up to date.\n", version.Version)
			return nil
		}
		fmt.Printf("Update %s prepared and handed off. This process will exit so the installed client can be replaced.\n", nextVersion)
		return nil
	case "apply-update":
		if strings.TrimSpace(candidatePath) == "" || strings.TrimSpace(updateVersion) == "" ||
			strings.TrimSpace(updateSHA) == "" || waitPID <= 0 {
			return errors.New("apply-update requires candidate, version, sha256 and a positive wait-pid")
		}
		return windowsclient.ApplyUserClientUpdate(windowsclient.DownloadedWindowsClientUpdate{
			Version: updateVersion,
			Path:    candidatePath,
			SHA256:  updateSHA,
		}, waitPID, configPath)
	}
	return nil
}

func startClientUpdate(ctx context.Context, restartAgentConfig string) (bool, string, error) {
	source := windowsclient.DefaultWindowsReleaseSource()
	update, err := source.FindUpdate(ctx, version.Version)
	if err != nil {
		return false, "", updateCheckError(err)
	}
	if update == nil {
		return false, "", nil
	}
	updateDir, err := windowsclient.UserClientUpdateDir()
	if err != nil {
		return false, "", err
	}
	downloaded, err := source.Download(ctx, *update, updateDir)
	if err != nil {
		return false, "", fmt.Errorf("download Windows client update %s: %w", update.Version, err)
	}
	if err := windowsclient.LaunchUserClientUpdate(downloaded, restartAgentConfig); err != nil {
		return false, "", fmt.Errorf("start Windows client update handoff: %w", err)
	}
	return true, update.Version, nil
}

func updateCheckError(err error) error {
	if errors.Is(err, windowsclient.ErrWindowsClientVersionUnknown) {
		return fmt.Errorf("this Windows client does not contain a release version; install a version-stamped release before checking for updates: %w", err)
	}
	return err
}

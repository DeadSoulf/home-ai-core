package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/windowsclient"
)

func runClient(args []string) error {
	if len(args) == 0 {
		return errors.New("client requires install, status or update")
	}
	command := args[0]
	switch command {
	case "install", "status", "update":
	case "help", "-h", "--help":
		printUsage()
		return nil
	default:
		return fmt.Errorf("unknown client command %q", command)
	}
	fs := flag.NewFlagSet("client "+command, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
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
		result, err := windowsclient.InstallUserClientWithHandoff(executable, 15*time.Second)
		if err != nil {
			return err
		}
		if result.AgentExitRequested {
			info, statusErr := windowsclient.UserAgentAutostartStatus()
			if statusErr != nil {
				return fmt.Errorf("read agent autostart after client update: %w", statusErr)
			}
			if info.Enabled {
				if err := windowsclient.StartUserAgent(result.Path, info.ConfigPath); err != nil {
					return fmt.Errorf("restart Home-AI sync agent: %w", err)
				}
				fmt.Println("Home-AI sync agent restarted after client update.")
			}
		}
		if result.Changed {
			fmt.Printf("Home-AI Windows client installed: %s\n", result.Path)
		} else {
			fmt.Printf("Home-AI Windows client is already current: %s\n", result.Path)
		}
		return nil
	case "update":
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer cancel()
		release, err := windowsclient.DiscoverLatestWindowsClientRelease(ctx, nil)
		if err != nil {
			return fmt.Errorf("discover latest Home-AI Windows client release: %w", err)
		}
		path, digest, err := windowsclient.DownloadWindowsClientRelease(ctx, nil, release)
		if err != nil {
			return fmt.Errorf("download Home-AI Windows client %s: %w", release.Version, err)
		}
		fmt.Printf("Verified Home-AI Windows client %s (SHA-256 %s).\n", release.Version, digest)
		if err := windowsclient.StartVerifiedClientInstall(path); err != nil {
			return err
		}
		fmt.Println("Verified update installer started. This process will exit so the installed client can be replaced safely.")
		return nil
	case "status":
		path, installed, err := windowsclient.UserClientInstallStatus()
		if err != nil {
			return err
		}
		if !installed {
			fmt.Printf("Home-AI Windows client is not installed. Expected path: %s\n", path)
			return nil
		}
		fmt.Printf("Home-AI Windows client installed: %s\n", path)
		return nil
	}
	return nil
}

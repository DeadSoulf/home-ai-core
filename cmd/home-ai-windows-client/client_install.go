package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/version"
	"github.com/DeadSoulf/home-ai-core/internal/windowsclient"
)

// runClientInstall runs in a verified cached executable (not the old running
// binary). Its default relaunch behavior is compatible with older updaters
// which invoke "client install" without flags.
func runClientInstall(reopenSettings bool, configPath, settingsPath string) (runErr error) {
	if reopenSettings {
		defer func() {
			if runErr != nil {
				notifyWindowsClientUpdateFailure(runErr)
			}
		}()
	}
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
	if reopenSettings {
		if err := reopenWindowsClientSettings(result.Path, configPath, settingsPath, result.Changed); err != nil {
			return err
		}
	}

	return nil
}

func runClient(args []string) error {
	if len(args) == 0 {
		return errors.New("client requires install, status, check-update or update")
	}
	command := args[0]
	switch command {
	case "install", "status", "check-update", "update":
	case "help", "-h", "--help":
		printUsage()
		return nil
	default:
		return fmt.Errorf("unknown client command %q", command)
	}
	fs := flag.NewFlagSet("client "+command, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var channel string
	var reopenSettings bool
	var configPath, settingsPath string
	if command == "check-update" || command == "update" {
		fs.StringVar(&channel, "channel", windowsclient.ReleaseChannelBeta, "release channel: stable or beta")
	}
	if command == "install" {
		// Old updater processes start a verified new release with only
		// "client install". Default to reopening the installed UI so updates
		// started by older clients also restore a visible application.
		fs.BoolVar(&reopenSettings, "reopen-settings", true, "open the installed Windows settings after installation")
		fs.StringVar(&configPath, "config", "", "sync profile path to reopen")
		fs.StringVar(&settingsPath, "settings", "", "Windows client settings path to reopen")
	}
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("client commands do not accept positional arguments")
	}

	switch command {
	case "install":
		return runClientInstall(reopenSettings, configPath, settingsPath)
	case "check-update", "update":
		var channelErr error
		channel, channelErr = windowsclient.NormalizeReleaseChannel(channel)
		if channelErr != nil {
			return channelErr
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer cancel()
		release, err := windowsclient.DiscoverWindowsClientRelease(ctx, nil, channel)
		if err != nil {
			return fmt.Errorf("discover latest Home-AI Windows client release: %w", err)
		}
		if command == "check-update" {
			fmt.Printf("Current version: %s\n%s channel latest: %s\n", version.Version, channel, release.Version)
			if windowsclient.CompareWindowsClientVersions(release.Version, version.Version) > 0 {
				fmt.Println("A newer version is available. Run client update --channel " + channel + " to install.")
			} else {
				fmt.Println("No newer release in the selected channel.")
			}
			return nil
		}
		if windowsclient.CompareWindowsClientVersions(release.Version, version.Version) <= 0 {
			fmt.Printf("No newer %s client release (current: %s, newest: %s).\n", channel, version.Version, release.Version)
			return nil
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

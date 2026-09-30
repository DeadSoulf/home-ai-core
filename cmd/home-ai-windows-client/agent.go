package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/windowsclient"
)

const maxAgentLogBytes int64 = 4 << 20

func defaultAgentLogPath() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		base, err = os.UserConfigDir()
	}
	if err != nil {
		return "", fmt.Errorf("locate user cache directory: %w", err)
	}
	return filepath.Join(base, "HomeAI", "sync-agent.log"), nil
}

func runAgent(args []string) error {
	if len(args) == 0 {
		return errors.New("agent requires install, status, remove or run")
	}
	command := args[0]
	switch command {
	case "install", "status", "remove", "run":
	case "help", "-h", "--help":
		printUsage()
		return nil
	default:
		return fmt.Errorf("unknown agent command %q", command)
	}

	fs := flag.NewFlagSet("agent "+command, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var configPath string
	fs.StringVar(&configPath, "config", "", "sync profile file (default: user configuration directory/HomeAI/sync-profiles.json)")
	var poll time.Duration
	var retries int
	var restartStale bool
	var logPath string
	if command == "run" {
		fs.DurationVar(&poll, "poll", 30*time.Second, "maximum delay before reloading profile configuration")
		fs.IntVar(&retries, "retries", 3, "retry count for transient chunk upload failures")
		fs.BoolVar(&restartStale, "restart-stale", false, "cancel mismatched unfinished uploads encountered by scheduled runs")
		fs.StringVar(&logPath, "log", "", "agent log file")
	}
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("agent commands do not accept positional arguments")
	}

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

	switch command {
	case "install":
		if err := validateAgentReady(configPath); err != nil {
			return err
		}
		executable, err := os.Executable()
		if err != nil {
			return fmt.Errorf("locate Windows client executable: %w", err)
		}
		result, err := windowsclient.InstallUserClientWithHandoff(executable, 15*time.Second)
		if err != nil {
			return fmt.Errorf("install Home-AI Windows client: %w", err)
		}
		if err := windowsclient.InstallUserAgentAutostart(result.Path, configPath); err != nil {
			return err
		}
		if result.AgentExitRequested {
			if err := windowsclient.StartUserAgent(result.Path, configPath); err != nil {
				return fmt.Errorf("restart Home-AI sync agent: %w", err)
			}
			fmt.Println("Home-AI sync agent restarted with the updated client.")
		}
		fmt.Printf("Home-AI Windows client installed: %s\n", result.Path)
		fmt.Println("Home-AI sync agent autostart enabled for the current Windows user.")
		fmt.Println("The agent will start after the user logs on.")
		return nil
	case "status":
		info, err := windowsclient.UserAgentAutostartStatus()
		if err != nil {
			return err
		}
		if !info.Enabled {
			fmt.Println("Home-AI sync agent autostart is disabled.")
			return nil
		}
		fmt.Println("Home-AI sync agent autostart is enabled.")
		fmt.Printf("Command: %s\n", info.Command)
		return nil
	case "remove":
		if err := windowsclient.RemoveUserAgentAutostart(); err != nil {
			return err
		}
		fmt.Println("Home-AI sync agent autostart removed.")
		return nil
	case "run":
		if poll < time.Second || poll > 5*time.Minute {
			return errors.New("--poll must be between 1s and 5m")
		}
		if retries < 1 {
			return errors.New("--retries must be positive")
		}
		if logPath == "" {
			var err error
			logPath, err = defaultAgentLogPath()
			if err != nil {
				return err
			}
		}
		lock, err := windowsclient.AcquireAgentLock(configPath + ".agent.lock")
		if err != nil {
			return err
		}
		defer lock.Close()
		cleanup, err := prepareAgentRuntime(logPath)
		if err != nil {
			return err
		}
		defer cleanup()
		fmt.Printf("%s Home-AI sync agent starting\n", time.Now().Format(time.RFC3339))
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
		defer cancel()
		tray, err := startAgentTray(logPath, configPath)
		if err != nil {
			return fmt.Errorf("start tray UI: %w", err)
		}
		defer tray.Close()
		if tray.Exit != nil {
			go func() {
				select {
				case <-tray.Exit:
					cancel()
				case <-ctx.Done():
				}
			}()
		}
		err = watchSyncProfiles(ctx, configPath, "", retries, restartStale, poll, tray.RunNow, tray.ReportCycle)
		fmt.Printf("%s Home-AI sync agent stopped: %v\n", time.Now().Format(time.RFC3339), err)
		return err
	}
	return nil
}

func validateAgentReady(configPath string) error {
	profiles, err := windowsclient.LoadSyncProfiles(configPath)
	if err != nil {
		return err
	}
	enabled := 0
	accounts := map[string]bool{}
	for _, profile := range profiles {
		if !profile.Enabled {
			continue
		}
		enabled++
		key := profile.ServerURL + "\x00" + profile.Username
		if accounts[key] {
			continue
		}
		accounts[key] = true
		if _, err := windowsclient.LoadPassword(profile.ServerURL, profile.Username); err != nil {
			if errors.Is(err, windowsclient.ErrCredentialNotFound) {
				return fmt.Errorf(
					"no stored Windows credential for %s at %s; run credentials save first",
					profile.Username,
					profile.ServerURL,
				)
			}
			return fmt.Errorf("check Windows credential for %s at %s: %w", profile.Username, profile.ServerURL, err)
		}
	}
	if enabled == 0 {
		return errors.New("no enabled sync profiles; add or enable a sync profile before installing the agent")
	}
	return nil
}

func openAgentLog(filename string) (*os.File, error) {
	filename = strings.TrimSpace(filename)
	if filename == "" {
		return nil, errors.New("agent log path is required")
	}
	filename, err := filepath.Abs(filename)
	if err != nil {
		return nil, fmt.Errorf("resolve agent log path: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(filename), 0o700); err != nil {
		return nil, fmt.Errorf("create agent log directory: %w", err)
	}
	if info, err := os.Lstat(filename); err == nil {
		if !info.Mode().IsRegular() {
			return nil, errors.New("agent log path must be a regular file")
		}
		if info.Size() > maxAgentLogBytes {
			_ = os.Remove(filename + ".1")
			if err := os.Rename(filename, filename+".1"); err != nil {
				return nil, fmt.Errorf("rotate agent log: %w", err)
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("inspect agent log: %w", err)
	}
	file, err := os.OpenFile(filename, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open agent log: %w", err)
	}
	return file, nil
}

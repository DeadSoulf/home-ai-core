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
	"text/tabwriter"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/windowsclient"
)

func defaultSyncConfigPath() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate user configuration directory: %w", err)
	}
	return filepath.Join(base, "HomeAI", "sync-profiles.json"), nil
}

func runSync(args []string) error {
	if len(args) == 0 {
		return errors.New("sync requires add, list, remove, enable, disable, run or watch")
	}
	command := args[0]
	if command == "help" || command == "--help" || command == "-h" {
		printUsage()
		return nil
	}
	switch command {
	case "add", "list", "remove", "enable", "disable", "run", "watch":
	default:
		return fmt.Errorf("unknown sync command %q", command)
	}

	fs := flag.NewFlagSet("sync "+command, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var configPath string
	fs.StringVar(&configPath, "config", "", "local sync profile file (default: user configuration directory/HomeAI/sync-profiles.json)")

	var auth authFlags
	var folderID, source, destination, profileID, conflict string
	var every, poll time.Duration
	var retries int
	var restartStale bool
	switch command {
	case "add":
		addAuthFlags(fs, &auth)
		fs.StringVar(&folderID, "folder", "", "target Home-AI folder ID")
		fs.StringVar(&source, "source", "", "local source file or directory")
		fs.StringVar(&destination, "dest", "", "relative destination path (default: source name)")
		fs.DurationVar(&every, "every", 15*time.Minute, "sync interval (minimum 1m)")
		fs.StringVar(&conflict, "conflict", windowsclient.SyncConflictStop, "conflict policy: stop, skip, replace-to-trash")
	case "remove", "enable", "disable":
		fs.StringVar(&profileID, "profile", "", "sync profile ID")
	case "run":
		fs.StringVar(&profileID, "profile", "", "run one profile now; without it, run enabled profiles that are due")
		fs.IntVar(&retries, "retries", 3, "retry count for transient chunk upload failures")
		fs.BoolVar(&restartStale, "restart-stale", false, "cancel mismatched unfinished uploads encountered by this run")
	case "watch":
		fs.StringVar(&profileID, "profile", "", "watch only one enabled profile")
		fs.IntVar(&retries, "retries", 3, "retry count for transient chunk upload failures")
		fs.BoolVar(&restartStale, "restart-stale", false, "cancel mismatched unfinished uploads encountered by scheduled runs")
		fs.DurationVar(&poll, "poll", 30*time.Second, "maximum delay before reloading profile configuration")
	}
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("sync commands do not accept positional arguments")
	}
	if configPath == "" {
		var err error
		configPath, err = defaultSyncConfigPath()
		if err != nil {
			return err
		}
	}

	switch command {
	case "add":
		if strings.TrimSpace(auth.server) == "" || strings.TrimSpace(auth.username) == "" ||
			strings.TrimSpace(folderID) == "" || strings.TrimSpace(source) == "" {
			return errors.New("--server, --username, --folder and --source are required")
		}
		profile, err := windowsclient.AddSyncProfile(configPath, windowsclient.SyncProfileInput{
			ServerURL:      auth.server,
			Username:       auth.username,
			FolderID:       folderID,
			Source:         source,
			Destination:    destination,
			Every:          every,
			ConflictPolicy: conflict,
		})
		if err != nil {
			return err
		}
		fmt.Printf("Sync profile added: %s\n", profile.ID)
		fmt.Printf("Every: %s\nConflict policy: %s\n", profile.Interval(), profile.ConflictPolicy)
		fmt.Printf("Source: %s\nDestination: %s\n", profile.Source, profile.Destination)
		return nil
	case "list":
		return printSyncProfiles(configPath)
	case "remove", "enable", "disable":
		if strings.TrimSpace(profileID) == "" {
			return errors.New("--profile is required")
		}
		var err error
		switch command {
		case "remove":
			err = windowsclient.RemoveSyncProfile(configPath, profileID)
		case "enable":
			err = windowsclient.SetSyncProfileEnabled(configPath, profileID, true)
		case "disable":
			err = windowsclient.SetSyncProfileEnabled(configPath, profileID, false)
		}
		if err != nil {
			return err
		}
		fmt.Printf("Sync profile %s: %s\n", profileID, command)
		return nil
	case "run":
		if retries < 1 {
			return errors.New("--retries must be positive")
		}
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
		defer cancel()
		count, err := executeSyncProfiles(ctx, configPath, profileID, profileID == "", retries, restartStale)
		if count == 0 && err == nil {
			fmt.Println("No sync profiles are due.")
		}
		return err
	case "watch":
		if retries < 1 {
			return errors.New("--retries must be positive")
		}
		if poll < time.Second || poll > 5*time.Minute {
			return errors.New("--poll must be between 1s and 5m")
		}
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
		defer cancel()
		return watchSyncProfiles(ctx, configPath, profileID, retries, restartStale, poll, nil, nil)
	}
	return nil
}

func watchSyncProfiles(
	ctx context.Context,
	configPath string,
	profileID string,
	retries int,
	restartStale bool,
	poll time.Duration,
	runNow <-chan struct{},
	onCycle func(manual bool, count int, err error),
) error {
	if retries < 1 {
		return errors.New("sync watcher retries must be positive")
	}
	if poll < time.Second || poll > 5*time.Minute {
		return errors.New("sync watcher poll must be between 1s and 5m")
	}
	if err := validateSyncWatchSelection(configPath, profileID); err != nil {
		return err
	}
	fmt.Println("Watching scheduled sync profiles. Press Ctrl+C to stop.")
	forceRun := false
	for {
		if ctx.Err() != nil {
			return nil
		}
		manual := forceRun
		count, runErr := executeSyncProfiles(ctx, configPath, profileID, !forceRun, retries, restartStale)
		forceRun = false
		if ctx.Err() != nil {
			return nil
		}
		if onCycle != nil && (manual || count > 0) {
			onCycle(manual, count, runErr)
		}
		if runErr != nil {
			fmt.Fprintln(os.Stderr, "sync cycle:", runErr)
		}
		delay, err := nextSyncWatchDelay(configPath, profileID, poll, time.Now().UTC())
		if err != nil {
			return err
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-runNow:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			forceRun = true
		case <-timer.C:
		}
	}
}

func printSyncProfiles(configPath string) error {
	profiles, err := windowsclient.LoadSyncProfiles(configPath)
	if err != nil {
		return err
	}
	if len(profiles) == 0 {
		fmt.Println("No sync profiles.")
		return nil
	}
	output := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(output, "PROFILE ID\tENABLED\tEVERY\tPOLICY\tLAST RESULT\tSOURCE -> DESTINATION")
	for _, profile := range profiles {
		last := "never"
		if profile.LastAttemptAt != nil {
			last = profile.LastAttemptAt.Local().Format(time.RFC3339)
			if profile.LastError != "" {
				last += " failed"
			} else {
				last += " ok"
			}
		}
		fmt.Fprintf(
			output,
			"%s\t%t\t%s\t%s\t%s\t%s -> %s\n",
			profile.ID,
			profile.Enabled,
			profile.Interval(),
			profile.ConflictPolicy,
			last,
			profile.Source,
			profile.Destination,
		)
		if profile.LastError != "" {
			fmt.Fprintf(output, "\t\t\t\t%s\n", profile.LastError)
		}
	}
	return output.Flush()
}

func executeSyncProfiles(
	ctx context.Context,
	configPath string,
	profileID string,
	dueOnly bool,
	retries int,
	restartStale bool,
) (int, error) {
	profiles, err := windowsclient.LoadSyncProfiles(configPath)
	if err != nil {
		return 0, err
	}
	selected, err := selectSyncProfiles(profiles, profileID, dueOnly, time.Now().UTC())
	if err != nil {
		return 0, err
	}
	if len(selected) == 0 {
		return 0, nil
	}
	var runErrors []error
	for _, profile := range selected {
		if err := ctx.Err(); err != nil {
			return len(selected), err
		}
		fmt.Printf("Sync %s: %s -> %s\n", profile.ID, profile.Source, profile.Destination)
		client, runErr := authenticatedClient(ctx, authFlags{
			server:   profile.ServerURL,
			username: profile.Username,
		})
		var summary windowsclient.SyncSummary
		if runErr == nil {
			summary, runErr = client.SyncOnce(ctx, profile.FolderID, profile.Source, profile.Destination, windowsclient.SyncOptions{
				ConflictPolicy: profile.ConflictPolicy,
				Upload: windowsclient.UploadOptions{
					Retries:      retries,
					RestartStale: restartStale,
				},
				Progress: func(progress windowsclient.SyncProgress) {
					if progress.Status == "unchanged" {
						return
					}
					fmt.Printf(
						"  [%d/%d] %s: %s\n",
						progress.Index,
						progress.Total,
						progress.Path,
						progress.Status,
					)
				},
			})
		}
		recordErr := windowsclient.RecordSyncProfileResult(configPath, profile.ID, time.Now().UTC(), runErr)
		if runErr == nil {
			fmt.Printf(
				"  complete: planned=%d copied=%d unchanged=%d conflicts=%d skipped=%d replaced=%d\n",
				summary.Planned,
				summary.Copied,
				summary.Unchanged,
				summary.Conflicts,
				summary.Skipped,
				summary.Replaced,
			)
		} else {
			fmt.Fprintf(os.Stderr, "  failed: %v\n", runErr)
			runErrors = append(runErrors, fmt.Errorf("%s: %w", profile.ID, runErr))
		}
		if recordErr != nil {
			runErrors = append(runErrors, fmt.Errorf("%s checkpoint: %w", profile.ID, recordErr))
		}
	}
	return len(selected), errors.Join(runErrors...)
}

func selectSyncProfiles(
	profiles []windowsclient.SyncProfile,
	profileID string,
	dueOnly bool,
	now time.Time,
) ([]windowsclient.SyncProfile, error) {
	profileID = strings.TrimSpace(profileID)
	if profileID != "" {
		for _, profile := range profiles {
			if profile.ID != profileID {
				continue
			}
			if dueOnly {
				if !profile.Enabled {
					return nil, nil
				}
				due := windowsclient.DueSyncProfiles([]windowsclient.SyncProfile{profile}, now)
				if len(due) == 0 {
					return nil, nil
				}
			}
			return []windowsclient.SyncProfile{profile}, nil
		}
		return nil, fmt.Errorf("sync profile %s not found", profileID)
	}
	if dueOnly {
		return windowsclient.DueSyncProfiles(profiles, now), nil
	}
	result := make([]windowsclient.SyncProfile, 0, len(profiles))
	for _, profile := range profiles {
		if profile.Enabled {
			result = append(result, profile)
		}
	}
	return result, nil
}

func validateSyncWatchSelection(configPath, profileID string) error {
	profiles, err := windowsclient.LoadSyncProfiles(configPath)
	if err != nil {
		return err
	}
	if strings.TrimSpace(profileID) != "" {
		for _, profile := range profiles {
			if profile.ID == profileID {
				if !profile.Enabled {
					return fmt.Errorf("sync profile %s is disabled", profileID)
				}
				return nil
			}
		}
		return fmt.Errorf("sync profile %s not found", profileID)
	}
	for _, profile := range profiles {
		if profile.Enabled {
			return nil
		}
	}
	return errors.New("no enabled sync profiles")
}

func nextSyncWatchDelay(configPath, profileID string, poll time.Duration, now time.Time) (time.Duration, error) {
	profiles, err := windowsclient.LoadSyncProfiles(configPath)
	if err != nil {
		return 0, err
	}
	if strings.TrimSpace(profileID) != "" {
		selected := make([]windowsclient.SyncProfile, 0, 1)
		for _, profile := range profiles {
			if profile.ID == profileID && profile.Enabled {
				selected = append(selected, profile)
				break
			}
		}
		profiles = selected
	}
	next, ok := windowsclient.NextSyncDue(profiles)
	if !ok {
		return poll, nil
	}
	if next.IsZero() || !now.Before(next) {
		return time.Second, nil
	}
	delay := next.Sub(now)
	if delay > poll {
		delay = poll
	}
	if delay < time.Second {
		delay = time.Second
	}
	return delay, nil
}

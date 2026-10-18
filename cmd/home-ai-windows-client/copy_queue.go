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

type copyFlags struct {
	auth         authFlags
	folder       string
	source       string
	destination  string
	restartStale bool
	retries      int
}

func addCopyFlags(fs *flag.FlagSet, flags *copyFlags) {
	addAuthFlags(fs, &flags.auth)
	fs.StringVar(&flags.folder, "folder", "", "target Home-AI folder ID")
	fs.StringVar(&flags.source, "source", "", "local source file or directory")
	fs.StringVar(&flags.destination, "dest", "", "relative destination path (default: source name)")
	fs.BoolVar(&flags.restartStale, "restart-stale", false, "cancel a mismatched unfinished upload for the same destination")
}

func validateCopyFlags(flags copyFlags) error {
	for _, value := range []struct{ name, value string }{
		{"server", flags.auth.server}, {"username", flags.auth.username},
		{"folder", flags.folder}, {"source", flags.source},
	} {
		if strings.TrimSpace(value.value) == "" {
			return fmt.Errorf("--%s is required", value.name)
		}
	}
	return nil
}

func runCopy(args []string) error {
	fs := flag.NewFlagSet("copy", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var flags copyFlags
	addCopyFlags(fs, &flags)
	fs.IntVar(&flags.retries, "retries", 3, "retry count for transient chunk upload failures")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("copy does not accept positional arguments")
	}
	if err := validateCopyFlags(flags); err != nil {
		return err
	}
	if flags.retries < 1 {
		return errors.New("--retries must be positive")
	}
	fmt.Println("Planning copy and checking source files...")
	transfers, err := windowsclient.PlanCopy(flags.source, flags.destination)
	if err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	client, err := authenticatedClient(ctx, flags.auth)
	if err != nil {
		return err
	}
	activityConfig, activityErr := defaultSyncConfigPath()
	activity := transferActivity{ProfileID: "copy", Status: "running", ItemsTotal: len(transfers)}
	for index, transfer := range transfers {
		fmt.Printf("[%d/%d] %s\n", index+1, len(transfers), transfer.Destination)
		activity.Path = transfer.Destination
		activity.ItemsDone = index
		activity.UploadedBytes = 0
		activity.TotalBytes = transfer.SizeBytes
		if activityErr == nil {
			_ = saveTransferProgress(activityConfig, activity)
		}
		options := windowsclient.UploadOptions{Retries: flags.retries, RestartStale: flags.restartStale}
		printProgress := printTransferProgress()
		lastActivityWrite := time.Time{}
		options.Progress = func(progress windowsclient.Progress) {
			printProgress(progress)
			if activityErr != nil || (time.Since(lastActivityWrite) < 500*time.Millisecond &&
				progress.UploadedBytes < progress.TotalBytes) {
				return
			}
			activity.UploadedBytes = progress.UploadedBytes
			activity.TotalBytes = progress.TotalBytes
			lastActivityWrite = time.Now()
			_ = saveTransferProgress(activityConfig, activity)
		}
		if _, err := client.CopyTransfer(ctx, flags.folder, transfer, options); err != nil {
			if activityErr == nil {
				activity.Status = "failed"
				activity.Error = err.Error()
				_ = saveTransferProgress(activityConfig, activity)
				_ = appendTransferHistory(activityConfig, activity)
			}
			return fmt.Errorf("copy %s: %w", transfer.Destination, err)
		}
		activity.ItemsDone = index + 1
		activity.Status = "copied"
		if activityErr == nil {
			_ = appendTransferHistory(activityConfig, activity)
		}
		activity.Status = "running"
	}
	activity.Status = "completed"
	if activityErr == nil {
		_ = saveTransferProgress(activityConfig, activity)
	}
	fmt.Printf("Copy complete: %d items.\n", len(transfers))
	return nil
}

func printTransferProgress() func(windowsclient.Progress) {
	lastPercent := -1
	return func(progress windowsclient.Progress) {
		percent := 100
		if progress.TotalBytes > 0 {
			percent = int(progress.UploadedBytes * 100 / progress.TotalBytes)
		}
		if percent == lastPercent {
			return
		}
		lastPercent = percent
		resume := ""
		if progress.Resumed {
			resume = " (resumed)"
		}
		fmt.Printf("  %s: %d%%%s\n", progress.Path, percent, resume)
	}
}

func defaultQueuePath() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate user configuration directory: %w", err)
	}
	return filepath.Join(base, "HomeAI", "transfer-queue.json"), nil
}

func runQueue(args []string) error {
	if len(args) == 0 {
		return errors.New("queue requires add, list, run or retry")
	}
	command := args[0]
	if command == "help" || command == "--help" || command == "-h" {
		printUsage()
		return nil
	}
	if command != "add" && command != "list" && command != "run" && command != "retry" {
		return fmt.Errorf("unknown queue command %q", command)
	}
	fs := flag.NewFlagSet("queue "+command, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var path string
	fs.StringVar(&path, "queue", "", "local queue file (default: user configuration directory/HomeAI/transfer-queue.json)")
	var flags copyFlags
	var jobID string
	var retries int
	switch command {
	case "add":
		addCopyFlags(fs, &flags)
	case "run":
		fs.IntVar(&retries, "retries", 3, "retry count for transient chunk upload failures")
	case "retry":
		fs.StringVar(&jobID, "job", "", "failed job ID from queue list")
	}
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("queue commands do not accept positional arguments")
	}
	if command == "add" {
		if err := validateCopyFlags(flags); err != nil {
			return err
		}
	}
	if command == "retry" && strings.TrimSpace(jobID) == "" {
		return errors.New("--job is required")
	}
	if command == "run" && retries < 1 {
		return errors.New("--retries must be positive")
	}
	if path == "" {
		var err error
		path, err = defaultQueuePath()
		if err != nil {
			return err
		}
	}
	queue, err := windowsclient.OpenQueue(path)
	if err != nil {
		return err
	}
	defer queue.Close()
	switch command {
	case "add":
		fmt.Println("Planning copy and checking source files...")
		job, err := queue.Add(flags.auth.server, flags.auth.username, flags.folder, flags.source, flags.destination, flags.restartStale)
		if err != nil {
			return err
		}
		fmt.Printf("Queued: %s (%d items)\nQueue: %s\n", job.ID, len(job.Transfers), path)
		return nil
	case "list":
		jobs := queue.Jobs()
		if len(jobs) == 0 {
			fmt.Println("Queue is empty.")
			return nil
		}
		output := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(output, "JOB ID\tSTATUS\tITEMS\tDESTINATION")
		for _, job := range jobs {
			fmt.Fprintf(output, "%s\t%s\t%d\t%s\n", job.ID, job.Status, len(job.Transfers), job.Destination)
			if job.Error != "" {
				fmt.Fprintf(output, "\t%s\n", job.Error)
			}
		}
		return output.Flush()
	case "retry":
		if err := queue.Retry(jobID); err != nil {
			return err
		}
		fmt.Printf("Job %s is pending. Run queue run to continue.\n", jobID)
		return nil
	case "run":
		jobs := queue.Jobs()
		if len(jobs) == 0 {
			fmt.Println("Queue is empty.")
			return nil
		}
		pending := false
		for _, job := range jobs {
			if job.Status == windowsclient.QueueFailed {
				return fmt.Errorf("job %s failed; use queue retry --job %s after addressing the error: %s", job.ID, job.ID, job.Error)
			}
			pending = pending || job.Status != windowsclient.QueueCompleted
		}
		if !pending {
			fmt.Println("Queue is already complete.")
			return nil
		}
		profile := queue.Profile()
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
		defer cancel()
		client, err := authenticatedClient(ctx, authFlags{server: profile.ServerURL, username: profile.Username})
		if err != nil {
			return err
		}
		lastItem, lastPercent := "", -1
		activityConfig, activityErr := defaultSyncConfigPath()
		lastUpdate := time.Time{}
		var lastActivity transferActivity
		err = queue.Run(ctx, client, windowsclient.UploadOptions{Retries: retries}, func(progress windowsclient.QueueProgress) {
			percent := 100
			if progress.TotalBytes > 0 {
				percent = int(progress.UploadedBytes * 100 / progress.TotalBytes)
			}
			if progress.TransferID == lastItem && percent == lastPercent && progress.Status == "running" {
				return
			}
			lastItem, lastPercent = progress.TransferID, percent
			fmt.Printf("[%d/%d] %s: %d%% %s\n", progress.CompletedTransfers, progress.TotalTransfers, progress.Path, percent, progress.Status)
			if activityErr != nil {
				return
			}
			status := progress.Status
			if status == windowsclient.QueueCompleted {
				status = "copied"
			}
			lastActivity = transferActivity{
				ProfileID: "queue:" + progress.JobID, Path: progress.Path,
				Status: status, UploadedBytes: progress.UploadedBytes, TotalBytes: progress.TotalBytes,
				ItemsDone: progress.CompletedTransfers, ItemsTotal: progress.TotalTransfers,
			}
			if progress.Status != windowsclient.QueueRunning || time.Since(lastUpdate) >= 500*time.Millisecond {
				_ = saveTransferProgress(activityConfig, lastActivity)
				lastUpdate = time.Now()
			}
			if status == "copied" || status == windowsclient.QueueFailed {
				_ = appendTransferHistory(activityConfig, lastActivity)
			}
		})
		if activityErr == nil {
			if err != nil {
				lastActivity.Status = "failed"
				lastActivity.Error = err.Error()
				_ = appendTransferHistory(activityConfig, lastActivity)
			} else {
				lastActivity.Status = "completed"
			}
			_ = saveTransferProgress(activityConfig, lastActivity)
		}
		if err != nil {
			return fmt.Errorf("queue stopped; saved progress is retained: %w", err)
		}
		fmt.Println("Queue complete.")
	}
	return nil
}

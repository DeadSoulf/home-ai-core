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

	"github.com/DeadSoulf/home-ai-core/internal/windowsclient"
)

const passwordEnv = "HOME_AI_PASSWORD"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		printUsage()
		return errors.New("command is required")
	}

	switch args[0] {
	case "folders":
		return runFolders(args[1:])
	case "upload":
		return runUpload(args[1:])
	case "copy":
		return runCopy(args[1:])
	case "queue":
		return runQueue(args[1:])
	case "sync":
		return runSync(args[1:])
	case "credentials":
		return runCredentials(args[1:])
	case "agent":
		return runAgent(args[1:])
	case "help", "-h", "--help":
		printUsage()
		return nil
	default:
		printUsage()
		return fmt.Errorf("unknown command %q", args[0])
	}
}

type authFlags struct {
	server   string
	username string
}

func addAuthFlags(fs *flag.FlagSet, flags *authFlags) {
	fs.StringVar(&flags.server, "server", "", "Home-AI server URL, for example http://192.168.1.10:8080")
	fs.StringVar(&flags.username, "username", "", "Home-AI username")
}

func authenticatedClient(ctx context.Context, flags authFlags) (*windowsclient.Client, error) {
	if strings.TrimSpace(flags.server) == "" {
		return nil, errors.New("--server is required")
	}
	if strings.TrimSpace(flags.username) == "" {
		return nil, errors.New("--username is required")
	}
	password, err := resolvePassword(flags)
	if err != nil {
		return nil, err
	}

	client, err := windowsclient.New(flags.server)
	if err != nil {
		return nil, err
	}
	if err := client.Login(ctx, flags.username, password); err != nil {
		return nil, fmt.Errorf("login: %w", err)
	}
	return client, nil
}

func resolvePassword(flags authFlags) (string, error) {
	if password := os.Getenv(passwordEnv); password != "" {
		return password, nil
	}
	password, err := windowsclient.LoadPassword(flags.server, flags.username)
	switch {
	case err == nil:
		return password, nil
	case errors.Is(err, windowsclient.ErrCredentialNotFound):
		return "", fmt.Errorf("%s is not set and no Windows Credential Manager password exists; run credentials save first", passwordEnv)
	case errors.Is(err, windowsclient.ErrCredentialStoreUnsupported):
		return "", fmt.Errorf("%s is required on this platform", passwordEnv)
	default:
		return "", fmt.Errorf("load stored Home-AI password: %w", err)
	}
}

func runFolders(args []string) error {
	fs := flag.NewFlagSet("folders", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var auth authFlags
	addAuthFlags(fs, &auth)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("folders does not accept positional arguments")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	client, err := authenticatedClient(ctx, auth)
	if err != nil {
		return err
	}
	folders, err := client.Folders(ctx)
	if err != nil {
		return fmt.Errorf("list folders: %w", err)
	}

	if len(folders) == 0 {
		fmt.Println("No accessible Home-AI folders.")
		return nil
	}
	fmt.Printf("%-38s  %-8s  %-5s  %s\n", "FOLDER ID", "TYPE", "WRITE", "NAME")
	for _, folder := range folders {
		fmt.Printf("%-38s  %-8s  %-5t  %s\n",
			folder.ID,
			folder.Kind,
			folder.CanWrite,
			folder.Name,
		)
	}
	return nil
}

func runUpload(args []string) error {
	fs := flag.NewFlagSet("upload", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var auth authFlags
	addAuthFlags(fs, &auth)

	var folderID string
	var source string
	var destination string
	var restartStale bool
	var retries int
	fs.StringVar(&folderID, "folder", "", "target Home-AI folder ID")
	fs.StringVar(&source, "source", "", "local source file")
	fs.StringVar(&destination, "dest", "", "relative destination path inside the Home-AI folder")
	fs.BoolVar(&restartStale, "restart-stale", false, "cancel a mismatched unfinished upload for the same destination")
	fs.IntVar(&retries, "retries", 3, "retry count for transient chunk upload failures")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("upload does not accept positional arguments")
	}
	if strings.TrimSpace(folderID) == "" {
		return errors.New("--folder is required")
	}
	if strings.TrimSpace(source) == "" {
		return errors.New("--source is required")
	}
	if destination == "" {
		destination = filepath.Base(source)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client, err := authenticatedClient(ctx, auth)
	if err != nil {
		return err
	}

	var lastPercent = -1
	result, err := client.UploadFile(ctx, folderID, source, windowsclient.UploadOptions{
		Destination:  destination,
		Retries:      retries,
		RestartStale: restartStale,
		Progress: func(progress windowsclient.Progress) {
			percent := 100
			if progress.TotalBytes > 0 {
				percent = int((progress.UploadedBytes * 100) / progress.TotalBytes)
			}
			if percent == lastPercent {
				return
			}
			lastPercent = percent
			resume := ""
			if progress.Resumed {
				resume = " (resumed)"
			}
			fmt.Printf("\rUploading %s: %3d%%%s", progress.Path, percent, resume)
			if percent >= 100 {
				fmt.Println()
			}
		},
	})
	if err != nil {
		return err
	}

	fmt.Printf("Uploaded: %s\n", result.Path)
	fmt.Printf("Size: %d bytes\n", result.SizeBytes)
	fmt.Printf("SHA-256: %s\n", result.SHA256)
	return nil
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "Home-AI Windows File Client")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Authentication uses HOME_AI_PASSWORD first; on Windows it can fall back to Credential Manager.")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Commands:")
	fmt.Fprintln(os.Stderr, "  folders --server URL --username USER")
	fmt.Fprintln(os.Stderr, "  upload  --server URL --username USER --folder ID --source FILE [--dest PATH] [--restart-stale]")
	fmt.Fprintln(os.Stderr, "  copy    --server URL --username USER --folder ID --source FILE_OR_DIRECTORY [--dest PATH]")
	fmt.Fprintln(os.Stderr, "  queue add   --server URL --username USER --folder ID --source FILE_OR_DIRECTORY [--dest PATH] [--queue FILE]")
	fmt.Fprintln(os.Stderr, "  queue list  [--queue FILE]")
	fmt.Fprintln(os.Stderr, "  queue run   [--queue FILE] [--retries COUNT]")
	fmt.Fprintln(os.Stderr, "  queue retry --job ID [--queue FILE]")
	fmt.Fprintln(os.Stderr, "  sync add     --server URL --username USER --folder ID --source FILE_OR_DIRECTORY [--dest PATH] [--every 15m] [--conflict stop|skip|replace-to-trash]")
	fmt.Fprintln(os.Stderr, "  sync list    [--config FILE]")
	fmt.Fprintln(os.Stderr, "  sync run     [--profile ID] [--config FILE]")
	fmt.Fprintln(os.Stderr, "  sync watch   [--profile ID] [--config FILE]")
	fmt.Fprintln(os.Stderr, "  sync enable|disable|remove --profile ID [--config FILE]")
	fmt.Fprintln(os.Stderr, "  credentials save|status|delete --server URL --username USER")
	fmt.Fprintln(os.Stderr, "  agent install [--config FILE]")
	fmt.Fprintln(os.Stderr, "  agent status")
	fmt.Fprintln(os.Stderr, "  agent remove")
	fmt.Fprintln(os.Stderr, "  agent run [--config FILE] [--poll 30s]")
}

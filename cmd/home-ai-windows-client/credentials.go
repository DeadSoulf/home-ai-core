package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/DeadSoulf/home-ai-core/internal/windowsclient"
)

func runCredentials(args []string) error {
	if len(args) == 0 {
		return errors.New("credentials requires save, status or delete")
	}
	command := args[0]
	switch command {
	case "save", "status", "delete":
	case "help", "-h", "--help":
		printUsage()
		return nil
	default:
		return fmt.Errorf("unknown credentials command %q", command)
	}

	fs := flag.NewFlagSet("credentials "+command, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var auth authFlags
	addAuthFlags(fs, &auth)
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("credentials commands do not accept positional arguments")
	}
	if strings.TrimSpace(auth.server) == "" || strings.TrimSpace(auth.username) == "" {
		return errors.New("--server and --username are required")
	}

	switch command {
	case "save":
		password := os.Getenv(passwordEnv)
		if password == "" {
			return fmt.Errorf("%s is required to save a credential", passwordEnv)
		}
		if err := windowsclient.SavePassword(auth.server, auth.username, password); err != nil {
			return err
		}
		target, _ := windowsclient.CredentialTarget(auth.server, auth.username)
		fmt.Printf("Saved Home-AI password in Windows Credential Manager.\nTarget: %s\n", target)
		return nil
	case "status":
		_, err := windowsclient.LoadPassword(auth.server, auth.username)
		if errors.Is(err, windowsclient.ErrCredentialNotFound) {
			fmt.Println("No stored Home-AI password.")
			return nil
		}
		if err != nil {
			return err
		}
		fmt.Println("Stored Home-AI password is available.")
		return nil
	case "delete":
		err := windowsclient.DeletePassword(auth.server, auth.username)
		if errors.Is(err, windowsclient.ErrCredentialNotFound) {
			fmt.Println("No stored Home-AI password.")
			return nil
		}
		if err != nil {
			return err
		}
		fmt.Println("Stored Home-AI password deleted.")
		return nil
	}
	return nil
}

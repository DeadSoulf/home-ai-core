package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/DeadSoulf/home-ai-core/internal/windowsclient"
)

func runClient(args []string) error {
	if len(args) == 0 {
		return errors.New("client requires install or status")
	}
	command := args[0]
	switch command {
	case "install", "status":
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
		path, err := windowsclient.InstallUserClient(executable)
		if err != nil {
			return err
		}
		fmt.Printf("Home-AI Windows client installed: %s\n", path)
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

// hwops-admin performs offline bootstrap and explicit legacy ownership mapping.
// Passwords are read from stdin, never command-line flags or output.
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"hwops/internal/adapters/postgres"
	"hwops/internal/identity"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	command := flag.String("command", "bootstrap", "bootstrap or map-legacy")
	username := flag.String("username", "", "account username")
	flag.Parse()
	if os.Getenv("HWOPS_DATABASE_URL") == "" || *username == "" {
		return fmt.Errorf("HWOPS_DATABASE_URL and -username are required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	store, err := postgres.Open(ctx, os.Getenv("HWOPS_DATABASE_URL"))
	if err != nil {
		return err
	}
	defer store.Close()
	switch *command {
	case "bootstrap":
		fmt.Fprintln(os.Stderr, "Read initial administrator password from stdin (12–72 bytes).")
		reader := bufio.NewReaderSize(os.Stdin, 128)
		line, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("password must be one newline-terminated line on stdin")
		}
		accounts, err := identity.New(store.Pool())
		if err != nil {
			return err
		}
		u, err := accounts.CreateUser(ctx, "", *username, strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r"), "ADMIN", true)
		if err != nil {
			return err
		}
		fmt.Printf("Administrator created: %s\n", u.Username)
	case "map-legacy":
		count, err := store.MapLegacyOwner(ctx, "local-operator", strings.ToLower(*username))
		if err != nil {
			return err
		}
		fmt.Printf("Mapped %d legacy conversations to %s\n", count, *username)
	default:
		return fmt.Errorf("unsupported command")
	}
	return nil
}

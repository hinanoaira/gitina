package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"gitina/internal/config"
	"gitina/internal/git/tiered"
	"gitina/internal/repo"
)

func runRepoCmd(args []string) {
	if len(args) == 0 {
		printRepoHelp()
		os.Exit(1)
	}

	sub := args[0]
	switch sub {
	case "ls", "list":
		handleRepoList(args[1:])
	case "rm", "delete":
		handleRepoRemove(args[1:])
	case "clean":
		handleRepoClean(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "Unknown sub-command: %s\n", sub)
		printRepoHelp()
		os.Exit(1)
	}
}

func printRepoHelp() {
	fmt.Println("Usage: gitina repo <command> [options]")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  ls, list           List all hosted repositories")
	fmt.Println("  rm, delete <path>  Delete a repository (moves to .trash by default)")
	fmt.Println("  clean              Purge old repositories from .trash")
	fmt.Println()
	fmt.Println("Options:")
	fmt.Println("  -c <config.yaml>   Path to configuration file (default: auto-detect ./config.yaml or /etc/gitina/config.yaml)")
	fmt.Println("  -f, --force        (for rm) Permanently delete without moving to trash")
	fmt.Println("  --days <N>         (for clean) Purge items older than N days (default: 30)")
}

func findConfigFile(flagVal string) string {
	if flagVal != "" && flagVal != "config.yaml" {
		return flagVal
	}
	if env := os.Getenv("GITINA_CONFIG"); env != "" {
		return env
	}
	if _, err := os.Stat("config.yaml"); err == nil {
		return "config.yaml"
	}
	if _, err := os.Stat("/etc/gitina/config.yaml"); err == nil {
		return "/etc/gitina/config.yaml"
	}
	return "config.yaml"
}

func loadRepoMgr(configPath string) (*repo.Manager, error) {
	resolved := findConfigFile(configPath)
	cfg, err := config.Load(resolved)
	if err != nil {
		return nil, fmt.Errorf("%w (searched path: %s)", err, resolved)
	}
	tieredMgr, err := tiered.NewManager(context.Background(), cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize storage manager: %w", err)
	}
	return repo.NewManager(cfg.Server.RepoRoot, tieredMgr), nil
}

func handleRepoList(args []string) {
	configPath := "config.yaml"
	fs := flag.NewFlagSet("repo ls", flag.ExitOnError)
	fs.StringVar(&configPath, "c", "config.yaml", "config path")
	_ = fs.Parse(args)

	mgr, err := loadRepoMgr(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()
	list, err := mgr.List(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing repositories: %v\n", err)
		os.Exit(1)
	}

	if len(list) == 0 {
		fmt.Println("No repositories found.")
		return
	}

	fmt.Printf("%-38s %-14s %-10s %s\n", "REPOSITORY", "STORAGE", "SIZE", "UPDATED AT")
	fmt.Println("----------------------------------------------------------------------------------------")
	for _, r := range list {
		storageStatus := r.Storage
		if r.Storage == "s3" {
			if r.IsCached {
				storageStatus = "s3 [CACHED]"
			} else {
				storageStatus = "s3 [S3-ONLY]"
			}
		}
		fmt.Printf("%-38s %-14s %-10s %s\n", r.Path, storageStatus, formatSize(r.DiskSize), r.UpdatedAt.Format("2006-01-02 15:04:05"))
	}
}

func handleRepoRemove(args []string) {
	configPath := "config.yaml"
	force := false

	fs := flag.NewFlagSet("repo rm", flag.ExitOnError)
	fs.StringVar(&configPath, "c", "config.yaml", "config path")
	fs.BoolVar(&force, "f", false, "force permanent delete")
	fs.BoolVar(&force, "force", false, "force permanent delete")
	_ = fs.Parse(args)

	extra := fs.Args()
	if len(extra) == 0 {
		fmt.Fprintln(os.Stderr, "Error: missing repository path")
		os.Exit(1)
	}
	repoPath := extra[0]

	mgr, err := loadRepoMgr(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()
	msg, err := mgr.Remove(ctx, repoPath, force)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Repository %q %s.\n", repoPath, msg)
}

func handleRepoClean(args []string) {
	configPath := "config.yaml"
	days := 30

	fs := flag.NewFlagSet("repo clean", flag.ExitOnError)
	fs.StringVar(&configPath, "c", "config.yaml", "config path")
	fs.IntVar(&days, "days", 30, "clean repositories older than N days")
	_ = fs.Parse(args)

	mgr, err := loadRepoMgr(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	cleaned, err := mgr.CleanTrash(days)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error cleaning trash: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Cleaned %d expired repository/repositories from .trash.\n", cleaned)
}

func formatSize(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}

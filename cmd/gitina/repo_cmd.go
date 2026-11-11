package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"text/tabwriter"

	"gitina/internal/config"
	"gitina/internal/repo"
)

func runRepoCmd(globalConfig string, args []string) {
	if len(args) == 0 {
		printRepoHelp()
		os.Exit(1)
	}

	sub := args[0]
	subArgs := args[1:]

	switch sub {
	case "ls", "list":
		handleRepoList(globalConfig, subArgs)
	case "rm", "delete":
		handleRepoRemove(globalConfig, subArgs)
	case "clean":
		handleRepoClean(globalConfig, subArgs)
	case "-h", "--help", "help":
		printRepoHelp()
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
	fmt.Println("  -c <config.yaml>   Path to configuration file (default: auto-detect)")
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
	candidates := []string{
		"config.yaml",
		"./config.yaml",
		"/etc/gitina/config.yaml",
		"/var/lib/gitina/config.yaml",
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return "config.yaml"
}

func loadRepoMgr(configPath string) (*repo.Manager, error) {
	resolved := findConfigFile(configPath)
	cfg, err := config.Load(resolved)
	if err != nil {
		return nil, fmt.Errorf("%w (searched path: %s)", err, resolved)
	}
	return repo.NewManager(cfg.Server.RepoRoot), nil
}

func handleRepoList(globalConfig string, args []string) {
	configPath := globalConfig
	if configPath == "" {
		configPath = "config.yaml"
	}
	fs := flag.NewFlagSet("repo ls", flag.ExitOnError)
	fs.StringVar(&configPath, "c", configPath, "config path")
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
		fmt.Printf("No repositories found under %s\n", mgr.RepoRoot())
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "REPOSITORY\tSTORAGE\tCACHED\tSIZE\tLAST MODIFIED")

	for _, r := range list {
		cachedStr := "-"
		if r.Storage != "local" {
			if r.IsCached {
				cachedStr = "yes"
			} else {
				cachedStr = "no"
			}
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			r.Path,
			r.Storage,
			cachedStr,
			formatBytes(r.DiskSize),
			r.UpdatedAt.Format("2006-01-02 15:04:05"),
		)
	}
	_ = w.Flush()
}

func handleRepoRemove(globalConfig string, args []string) {
	configPath := globalConfig
	if configPath == "" {
		configPath = "config.yaml"
	}
	force := false

	fs := flag.NewFlagSet("repo rm", flag.ExitOnError)
	fs.StringVar(&configPath, "c", configPath, "config path")
	fs.BoolVar(&force, "f", false, "force permanent delete without moving to trash")
	fs.BoolVar(&force, "force", false, "force permanent delete without moving to trash")
	_ = fs.Parse(args)

	targets := fs.Args()
	if len(targets) == 0 {
		fmt.Fprintln(os.Stderr, "Error: repository path is required (e.g. 'owner/repo.git')")
		os.Exit(1)
	}

	mgr, err := loadRepoMgr(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()
	for _, targetRepo := range targets {
		msg, err := mgr.Remove(ctx, targetRepo, force)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error deleting repository %q: %v\n", targetRepo, err)
		} else {
			fmt.Printf("%s: %s\n", targetRepo, msg)
		}
	}
}

func handleRepoClean(globalConfig string, args []string) {
	configPath := globalConfig
	if configPath == "" {
		configPath = "config.yaml"
	}
	days := 30

	fs := flag.NewFlagSet("repo clean", flag.ExitOnError)
	fs.StringVar(&configPath, "c", configPath, "config path")
	fs.IntVar(&days, "days", 30, "purge trash items older than N days")
	_ = fs.Parse(args)

	mgr, err := loadRepoMgr(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	deleted, err := mgr.CleanTrash(days)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error cleaning trash: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Purged %d repository(ies) older than %d days from trash.\n", deleted, days)
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

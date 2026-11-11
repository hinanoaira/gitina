package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"gitina/internal/acl"
	"gitina/internal/auth"
	"gitina/internal/config"
	"gitina/internal/git"
	"gitina/internal/lfs"
	"gitina/internal/repo"
	"gitina/internal/web"
)

const version = "0.1.0"

func main() {
	if len(os.Args) <= 1 {
		printGlobalHelp()
		return
	}

	var globalConfig string
	var remainingArgs []string

	for i := 1; i < len(os.Args); i++ {
		arg := os.Args[i]
		if arg == "-c" || arg == "--config" {
			if i+1 < len(os.Args) {
				globalConfig = os.Args[i+1]
				i++
			}
		} else if strings.HasPrefix(arg, "-c=") {
			globalConfig = strings.TrimPrefix(arg, "-c=")
		} else if strings.HasPrefix(arg, "--config=") {
			globalConfig = strings.TrimPrefix(arg, "--config=")
		} else if arg == "-h" || arg == "--help" || arg == "help" {
			printGlobalHelp()
			return
		} else if arg == "-v" || arg == "--version" || arg == "version" {
			printVersion()
			return
		} else {
			remainingArgs = append(remainingArgs, arg)
		}
	}

	if len(remainingArgs) == 0 {
		printGlobalHelp()
		return
	}

	subCmd := remainingArgs[0]
	subArgs := remainingArgs[1:]

	switch subCmd {
	case "repo":
		runRepoCmd(globalConfig, subArgs)
	case "serve":
		runServeCmd(globalConfig, subArgs)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", subCmd)
		printGlobalHelp()
		os.Exit(1)
	}
}

func printGlobalHelp() {
	fmt.Println("gitina - Lightweight Git & LFS server")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  gitina [global options] <command> [arguments]")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  serve      Start the Git HTTP and LFS server")
	fmt.Println("  repo       Manage repositories (ls, rm, clean)")
	fmt.Println("  version    Show version information")
	fmt.Println("  help       Show this help message")
	fmt.Println()
	fmt.Println("Global Options:")
	fmt.Println("  -c <path>  Path to configuration file (default: auto-detect)")
	fmt.Println("  -h, --help Show help information")
	fmt.Println()
	fmt.Println("Run 'gitina <command> -h' for more information on a command.")
}

func printVersion() {
	fmt.Printf("gitina v%s\n", version)
}

func runServeCmd(globalConfig string, args []string) {
	configPath := globalConfig
	if configPath == "" {
		configPath = "config.yaml"
	}

	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	fs.StringVar(&configPath, "c", configPath, "path to config file")
	_ = fs.Parse(args)

	resolvedPath := findConfigFile(configPath)
	cfg, err := config.Load(resolvedPath)
	if err != nil {
		log.Fatalf("Failed to load configuration from %q: %v", resolvedPath, err)
	}

	// 1. ACL Manager
	aclMgr := acl.NewManager(cfg.Admins, cfg.Repositories)
	aclMgr.SetRepoRoot(cfg.Server.RepoRoot)

	// 2. Auth & GCM Handlers
	tokenMgr := auth.NewTokenManager(cfg.Server.JWTSecret)
	patStorePath := filepath.Join(cfg.Server.RepoRoot, ".tokens.json")
	if patStore, err := auth.NewPATStore(patStorePath); err == nil {
		tokenMgr.SetPATStore(patStore)
	} else {
		log.Printf("Warning: failed to initialize PAT store: %v", err)
	}
	gcmHandler := auth.NewGCMHandler(cfg, tokenMgr, aclMgr)

	// 3. Storage Backend for LFS
	var storage lfs.StorageBackend
	if cfg.LFS.Storage == "s3" {
		s3Storage, err := lfs.NewS3Storage(context.Background(), cfg.LFS.S3)
		if err != nil {
			log.Fatalf("Failed to initialize S3 storage for LFS: %v", err)
		}
		storage = s3Storage
		log.Println("[LFS] Initialized S3-compatible storage backend")
	} else {
		localStorage, err := lfs.NewLocalStorage(cfg.LFS.Local.Path)
		if err != nil {
			log.Fatalf("Failed to initialize local storage for LFS: %v", err)
		}
		storage = localStorage
		log.Printf("[LFS] Initialized local storage backend at %q", cfg.LFS.Local.Path)
	}

	lfsHandler := lfs.NewHandler(cfg, storage, aclMgr, gcmHandler)

	// LFS Automatic Garbage Collector Worker
	lfsGC := lfs.NewGarbageCollector(cfg.Server.RepoRoot, storage, cfg.LFS.GC)
	lfsGC.StartWorker(context.Background())

	// 4. Repository Manager & Web UI Handler
	repoMgr := repo.NewManager(cfg.Server.RepoRoot)
	webHandler, err := web.NewHandler(cfg, aclMgr, tokenMgr, repoMgr)
	if err != nil {
		log.Fatalf("Failed to initialize web UI handler: %v", err)
	}

	// 5. Git Smart HTTP Handler
	gitHandler, err := git.NewHandler(cfg, aclMgr, gcmHandler, lfsHandler)
	if err != nil {
		log.Fatalf("Failed to initialize git handler: %v", err)
	}

	// 7. Root Multiplexer
	mux := http.NewServeMux()

	// GCM / Gitea OAuth2 endpoints
	mux.HandleFunc("/login/oauth/authorize", gcmHandler.HandleAuthorize)
	mux.HandleFunc("/login/oauth/github/callback", gcmHandler.HandleGitHubCallback)
	mux.HandleFunc("/login/oauth/access_token", gcmHandler.HandleToken)
	mux.HandleFunc("/login/oauth/logout", gcmHandler.HandleLogout)
	mux.HandleFunc("/api/v1/user", gcmHandler.HandleUser)
	mux.HandleFunc("/api/v1/version", gcmHandler.HandleVersion)

	// Intelligent Root Router: dispatches to Git Smart HTTP or Web UI
	rootRouter := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		isGit := false
		ua := r.Header.Get("User-Agent")
		accept := r.Header.Get("Accept")

		if strings.HasPrefix(ua, "git/") || strings.HasPrefix(ua, "git-lfs") {
			isGit = true
		} else if r.URL.Query().Get("service") != "" {
			isGit = true
		} else if strings.HasSuffix(r.URL.Path, "/git-upload-pack") || strings.HasSuffix(r.URL.Path, "/git-receive-pack") || strings.HasSuffix(r.URL.Path, "/info/refs") || strings.Contains(r.URL.Path, "/info/lfs/") {
			isGit = true
		}

		if isGit || (!strings.Contains(accept, "text/html") && !strings.HasPrefix(r.URL.Path, "/api/") && r.Method == http.MethodPost) {
			gitHandler.ServeHTTP(w, r)
			return
		}

		// Web API access
		if strings.HasPrefix(r.URL.Path, "/api/v1/") {
			if webHandler.ServeAPI(w, r) {
				return
			}
		}

		// Web UI access
		if r.URL.Path == "/" {
			webHandler.ServeHome(w, r)
			return
		}

		webHandler.ServeHTTP(w, r)
	})

	mux.Handle("/", rootRouter)

	log.Printf("Starting gitina server on %s (Base URL: %s)...", cfg.Server.Listen, cfg.Server.BaseURL)
	if err := http.ListenAndServe(cfg.Server.Listen, mux); err != nil {
		log.Fatalf("Server stopped: %v", err)
	}
}

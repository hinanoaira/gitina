package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"strings"

	"gitina/internal/acl"
	"gitina/internal/auth"
	"gitina/internal/config"
	"gitina/internal/git"
	"gitina/internal/lfs"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "repo" {
		runRepoCmd(os.Args[2:])
		return
	}

	configPath := "config.yaml"
	serveCmd := flag.NewFlagSet("serve", flag.ExitOnError)
	serveCmd.StringVar(&configPath, "c", "config.yaml", "path to config file")

	if len(os.Args) > 1 && os.Args[1] == "serve" {
		_ = serveCmd.Parse(os.Args[2:])
	} else if len(os.Args) > 1 && strings.HasPrefix(os.Args[1], "-") {
		_ = serveCmd.Parse(os.Args[1:])
	}

	resolvedPath := findConfigFile(configPath)
	cfg, err := config.Load(resolvedPath)
	if err != nil {
		log.Fatalf("Failed to load configuration from %q: %v", resolvedPath, err)
	}

	// 1. ACL Manager
	aclMgr := acl.NewManager(cfg.Admins, cfg.Repositories)

	// 2. Auth & GCM Handlers
	tokenMgr := auth.NewTokenManager(cfg.Server.JWTSecret)
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

	// 4. Git Smart HTTP Handler
	gitHandler, err := git.NewHandler(cfg, aclMgr, gcmHandler, lfsHandler)
	if err != nil {
		log.Fatalf("Failed to initialize git handler: %v", err)
	}

	// 5. Root Multiplexer
	mux := http.NewServeMux()

	// GCM / Gitea OAuth2 endpoints
	mux.HandleFunc("/login/oauth/authorize", gcmHandler.HandleAuthorize)
	mux.HandleFunc("/login/oauth/github/callback", gcmHandler.HandleGitHubCallback)
	mux.HandleFunc("/login/oauth/access_token", gcmHandler.HandleToken)
	mux.HandleFunc("/api/v1/user", gcmHandler.HandleUser)
	mux.HandleFunc("/api/v1/version", gcmHandler.HandleVersion)

	// Git Smart HTTP & LFS fallback handler (handles all arbitrary repo paths)
	mux.Handle("/", gitHandler)

	log.Printf("Starting gitina server on %s (Base URL: %s)...", cfg.Server.Listen, cfg.Server.BaseURL)
	if err := http.ListenAndServe(cfg.Server.Listen, mux); err != nil {
		log.Fatalf("Server stopped: %v", err)
	}
}

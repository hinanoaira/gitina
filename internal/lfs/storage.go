package lfs

import (
	"context"
	"net/http"
)

type StorageBackend interface {
	// Exists checks if the object already exists in storage and matches the size.
	Exists(ctx context.Context, repoPath, oid string, size int64) (bool, error)

	// GetUploadAction returns the upload action and optional verify action for an object.
	GetUploadAction(ctx context.Context, baseURL, repoPath, oid string, size int64) (uploadAction *ActionLink, verifyAction *ActionLink, err error)

	// GetDownloadAction returns the download action for an object.
	GetDownloadAction(ctx context.Context, baseURL, repoPath, oid string, size int64) (downloadAction *ActionLink, err error)

	// VerifyObject verifies that an object was uploaded successfully (e.g. for S3).
	VerifyObject(ctx context.Context, repoPath, oid string, size int64) error

	// ServeObject (for local storage) handles HTTP GET to stream the object to the client.
	ServeObject(w http.ResponseWriter, r *http.Request, repoPath, oid string)

	// PutObject (for local storage) handles HTTP PUT to store the object from the client.
	PutObject(w http.ResponseWriter, r *http.Request, repoPath, oid string)
}

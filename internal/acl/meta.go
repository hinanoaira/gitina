package acl

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

const MetadataFileName = "gitina-meta.json"

type RepoMetadata struct {
	Public        bool              `json:"public"`
	Visibility    string            `json:"visibility,omitempty"` // "public", "unlisted", "private"
	Collaborators map[string]string `json:"collaborators"`        // username -> "ro" | "rw"
	UpdatedAt     time.Time         `json:"updated_at"`
	UpdatedBy     string            `json:"updated_by"`
}

// EffectiveVisibility resolves the normalized visibility: "public", "unlisted", or "private".
func (m *RepoMetadata) EffectiveVisibility() string {
	if m == nil {
		return "private"
	}
	if m.Visibility != "" {
		return m.Visibility
	}
	if m.Public {
		return "public"
	}
	return "private"
}

// LoadMetadata reads gitina-meta.json from repoDir. Returns nil, nil if the file does not exist.
func LoadMetadata(repoDir string) (*RepoMetadata, error) {
	filePath := filepath.Join(repoDir, MetadataFileName)
	data, err := os.ReadFile(filePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}

	var meta RepoMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}
	if meta.Collaborators == nil {
		meta.Collaborators = make(map[string]string)
	}
	return &meta, nil
}

// SaveMetadata writes gitina-meta.json atomically to repoDir.
func SaveMetadata(repoDir string, meta *RepoMetadata) error {
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		return err
	}
	if meta.Collaborators == nil {
		meta.Collaborators = make(map[string]string)
	}
	meta.UpdatedAt = time.Now()

	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}

	tmpFile := filepath.Join(repoDir, MetadataFileName+".tmp")
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return err
	}

	targetFile := filepath.Join(repoDir, MetadataFileName)
	return os.Rename(tmpFile, targetFile)
}

package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

type FilePreference struct {
	Enabled       bool   `json:"enabled"`
	LastKnownPath string `json:"lastKnownPath"`
}

type storedPreferences struct {
	Files map[string]FilePreference `json:"files"`
}

// PathKeyPrefix marks keys derived from the path alone, used when the file's
// identity cannot be read.
const PathKeyPrefix = "path:"

// PerFileStore remembers the auto-save switch of every document. Entries are
// keyed by file identity (volume serial + file ID on Windows), so the setting
// follows a document that is renamed or moved on the same volume, like the
// macOS version. If the identity changes — e.g. the file was replaced on save —
// the entry is found again through its last known path.
type PerFileStore struct {
	storagePath      string
	keyFor           func(path string) string
	initiallyEnabled []string
	preferences      storedPreferences
}

func NewPerFileStore(storagePath string, keyFor func(path string) string, initiallyEnabledPaths []string) *PerFileStore {
	store := &PerFileStore{
		storagePath:      storagePath,
		keyFor:           keyFor,
		initiallyEnabled: initiallyEnabledPaths,
		preferences:      storedPreferences{Files: map[string]FilePreference{}},
	}
	if data, err := os.ReadFile(storagePath); err == nil {
		var decoded storedPreferences
		if json.Unmarshal(data, &decoded) == nil && decoded.Files != nil {
			store.preferences = decoded
		}
	}
	return store
}

func (s *PerFileStore) IsEnabled(path string) bool {
	key := s.keyFor(path)
	if stored, ok := s.preferences.Files[key]; ok {
		if !samePath(stored.LastKnownPath, path) {
			// Renamed or moved: remember where the document lives now.
			stored.LastKnownPath = path
			s.preferences.Files[key] = stored
			_ = s.save()
		}
		return stored.Enabled
	}

	if oldKey, stored, ok := s.findByPath(path); ok {
		if !strings.HasPrefix(key, PathKeyPrefix) {
			delete(s.preferences.Files, oldKey)
			stored.LastKnownPath = path
			s.preferences.Files[key] = stored
			_ = s.save()
		}
		return stored.Enabled
	}

	for _, initialPath := range s.initiallyEnabled {
		if samePath(initialPath, path) || s.keyFor(initialPath) == key {
			return true
		}
	}
	return false
}

func (s *PerFileStore) SetEnabled(enabled bool, path string) error {
	key := s.keyFor(path)
	if oldKey, _, ok := s.findByPath(path); ok && oldKey != key {
		delete(s.preferences.Files, oldKey)
	}
	s.preferences.Files[key] = FilePreference{Enabled: enabled, LastKnownPath: path}
	return s.save()
}

func (s *PerFileStore) findByPath(path string) (string, FilePreference, bool) {
	for key, stored := range s.preferences.Files {
		if samePath(stored.LastKnownPath, path) {
			return key, stored, true
		}
	}
	return "", FilePreference{}, false
}

func (s *PerFileStore) save() error {
	if err := os.MkdirAll(filepath.Dir(s.storagePath), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s.preferences, "", "  ")
	if err != nil {
		return err
	}
	temporary := s.storagePath + ".tmp"
	if err := os.WriteFile(temporary, append(data, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(temporary, s.storagePath)
}

// samePath compares paths the way Windows does: case-insensitively and
// regardless of slash direction.
func samePath(a, b string) bool {
	return a != "" && strings.EqualFold(toBackslashes(a), toBackslashes(b))
}

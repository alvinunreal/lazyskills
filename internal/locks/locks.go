package locks

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/alvinunreal/lazyskills/internal/model"
)

func ProjectLockPath(cwd string) string {
	return filepath.Join(cwd, "skills-lock.json")
}

func GlobalLockPath() string {
	if v := os.Getenv("XDG_STATE_HOME"); v != "" {
		return filepath.Join(v, "skills", ".skill-lock.json")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".agents", ".skill-lock.json")
}

func ReadLocal(path string) (model.LocalLockFile, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return model.LocalLockFile{Version: 1, Skills: map[string]model.LocalLockEntry{}}, err
	}
	var lock model.LocalLockFile
	if err := json.Unmarshal(b, &lock); err != nil || lock.Version < 1 || lock.Skills == nil {
		if err == nil {
			err = os.ErrInvalid
		}
		return model.LocalLockFile{Version: 1, Skills: map[string]model.LocalLockEntry{}}, err
	}
	return lock, nil
}

func ReadGlobal(path string) (model.GlobalLockFile, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return model.GlobalLockFile{Version: 3, Skills: map[string]model.GlobalLockEntry{}}, err
	}
	var lock model.GlobalLockFile
	if err := json.Unmarshal(b, &lock); err != nil || lock.Version < 3 || lock.Skills == nil {
		if err == nil {
			err = os.ErrInvalid
		}
		return model.GlobalLockFile{Version: 3, Skills: map[string]model.GlobalLockEntry{}}, err
	}
	return lock, nil
}

// RemoveEntry deletes a single skill entry by key from a lock file, preserving
// all other entries and every top-level field, then rewrites it as indented
// JSON. It errors if the file, its skills map, or the key is missing. Decoding
// into a generic map (rather than the typed structs) avoids dropping fields the
// official CLI may add that this tool does not model.
func RemoveEntry(path, key string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var root map[string]any
	if err := json.Unmarshal(b, &root); err != nil {
		return err
	}
	skills, ok := root["skills"].(map[string]any)
	if !ok {
		return fmt.Errorf("lock file %s has no skills map", path)
	}
	if _, exists := skills[key]; !exists {
		return fmt.Errorf("lock entry %q not found", key)
	}
	delete(skills, key)
	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(out, '\n'), 0o644)
}

// RemoveEntryIfExists deletes a lock entry when both the lock file and key are
// present. Missing files and absent keys are treated as no-ops, making it safe
// to run after external uninstall commands that may or may not update locks
// themselves.
func RemoveEntryIfExists(path, key string) (bool, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	var root map[string]any
	if err := json.Unmarshal(b, &root); err != nil {
		return false, err
	}
	skills, ok := root["skills"].(map[string]any)
	if !ok {
		return false, fmt.Errorf("lock file %s has no skills map", path)
	}
	if _, exists := skills[key]; !exists {
		return false, nil
	}
	delete(skills, key)
	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return false, err
	}
	return true, os.WriteFile(path, append(out, '\n'), 0o644)
}

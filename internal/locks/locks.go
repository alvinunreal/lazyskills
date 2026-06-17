package locks

import (
	"encoding/json"
	"os"
	"path/filepath"

	"lazyskills/internal/model"
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

package security

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/Oranjekop/GistManager/internal/storage"
)

// ImportLegacy preserves the old files, including Chromium's encryption key.
// The native copy records where to read that key until its token is migrated.
func ImportLegacy(directory, configRoot string) error {
	if _, err := os.Stat(filepath.Join(directory, "security.json")); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	for _, name := range []string{"gist-editor-workspace", "Gist管理器"} {
		legacy := filepath.Join(configRoot, name)
		if filepath.Clean(legacy) == filepath.Clean(directory) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(legacy, "security.json"))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		var settings Settings
		if err = json.Unmarshal(data, &settings); err != nil {
			return err
		}
		settings.LegacyDirectory = legacy
		if err = storage.WriteJSON(filepath.Join(directory, "security.json"), settings); err != nil {
			return err
		}
		if updater, err := os.ReadFile(filepath.Join(legacy, "updater.json")); err == nil {
			if _, exists := os.Stat(filepath.Join(directory, "updater.json")); errors.Is(exists, os.ErrNotExist) {
				if err = os.WriteFile(filepath.Join(directory, "updater.json"), updater, 0o600); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return nil
}

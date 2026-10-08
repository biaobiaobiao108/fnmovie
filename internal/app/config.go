package app

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/egoist/mygo"
)

func configureCompatibleAppPaths() {
	// MyGo derives these paths from its display name by default. Preserve the
	// existing native window state, cache and logs when the brand changes.
	if dir, err := os.UserConfigDir(); err == nil {
		mygo.App.SetPath(mygo.PathUserData, filepath.Join(dir, "FnMovie"))
		mygo.App.SetPath(mygo.PathLogs, filepath.Join(dir, "FnMovie", "logs"))
	}
	if dir, err := os.UserCacheDir(); err == nil {
		mygo.App.SetPath(mygo.PathCache, filepath.Join(dir, "FnMovie"))
	}
}

type Settings struct {
	ServerURL string `json:"serverUrl"`
	Username  string `json:"username,omitempty"`
}

func settingsPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir = filepath.Join(dir, "FnMovie")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	return filepath.Join(dir, "settings.json"), nil
}

func LoadSettings() (Settings, error) {
	filename, err := settingsPath()
	if err != nil {
		return Settings{}, err
	}
	data, err := os.ReadFile(filename)
	if os.IsNotExist(err) {
		return Settings{}, nil
	}
	if err != nil {
		return Settings{}, err
	}
	var settings Settings
	err = json.Unmarshal(data, &settings)
	return settings, err
}

func SaveSettings(settings Settings) error {
	filename, err := settingsPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filename, data, 0600)
}

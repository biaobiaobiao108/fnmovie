package app

import (
	"encoding/json"
	"os"
	"path/filepath"
)

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

package app

import "testing"

func TestSettingsStartWithoutPersonalServer(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AppData", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	settings, err := LoadSettings()
	if err != nil || settings.ServerURL != "" || settings.Username != "" {
		t.Fatalf("first run should request a server: settings=%+v err=%v", settings, err)
	}
	if err := SaveSettings(Settings{Username: "user"}); err != nil {
		t.Fatal(err)
	}
	settings, err = LoadSettings()
	if err != nil || settings.ServerURL != "" || settings.Username != "user" {
		t.Fatalf("empty saved server should remain empty: settings=%+v err=%v", settings, err)
	}
	want := Settings{ServerURL: "http://nas.example/v", Username: "user"}
	if err := SaveSettings(want); err != nil {
		t.Fatal(err)
	}
	settings, err = LoadSettings()
	if err != nil || settings != want {
		t.Fatalf("saved server did not round trip: settings=%+v err=%v", settings, err)
	}
}

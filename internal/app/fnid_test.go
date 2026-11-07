package app

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestNormalizeFNIDInput(t *testing.T) {
	cases := map[string]string{
		" abcd1234 ":                  "abcd1234",
		"https://5ddd.com/abcd1234":   "abcd1234",
		"https://5ddd.com/abcd1234/v": "abcd1234",
		"5ddd.com/abcd1234/":          "abcd1234",
		"ABCD1234":                    "abcd1234",
	}
	for input, want := range cases {
		if got := normalizeFNIDInput(input); got != want {
			t.Errorf("normalizeFNIDInput(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestIsValidFNID(t *testing.T) {
	for _, value := range []string{"abcd1234", "a12345", "fn-connect-01"} {
		if !isValidFNID(value) {
			t.Errorf("%q should be a valid FN ID", value)
		}
	}
	for _, value := range []string{"", "abc", "1abcd123", "abcd_123", "abcd--123", "abcd123-", "abcd.1234", "abcdefghijklmnopqrstuvwxyz12345"} {
		if isValidFNID(value) {
			t.Errorf("%q should be invalid", value)
		}
	}
}

func TestFNConnectHeaders(t *testing.T) {
	body := []byte(`{"fnId":"test123"}`)
	authx, fnSign := fnConnectHeaders("test123", 1700000000000, "123456", body)
	if authx != "nonce=123456&timestamp=1700000000000&sign=5994647128554c366ec937f6a536b0ef" {
		t.Fatalf("authx = %q", authx)
	}
	if fnSign != "f6d2f0c41c325f77900b2a143408c0917c8f44f0f8b82d5e45eb9a702917eb6b" {
		t.Fatalf("fn-sign = %q", fnSign)
	}
}

func TestFNConnectServerURLPrefersRelay(t *testing.T) {
	data := map[string]any{
		"port":       map[string]any{"httpPort": float64(5666), "httpsPort": float64(8443)},
		"publicIpv4": []any{"1.2.3.4"},
		"publicIpv6": []any{"2001:db8::1"},
		"fn":         []any{"relay.example.com"},
		"ddns":       []any{"home.example.com"},
	}
	result, ok := fnConnectServerURL(data)
	if !ok || !result.Relay || result.ServerURL != "https://relay.example.com/v" {
		t.Fatalf("relay result = %+v ok=%v", result, ok)
	}

	delete(data, "fn")
	result, ok = fnConnectServerURL(data)
	if !ok || result.Relay || result.ServerURL != "http://1.2.3.4:5666/v" {
		t.Fatalf("ipv4 result = %+v ok=%v", result, ok)
	}

	delete(data, "publicIpv4")
	result, ok = fnConnectServerURL(data)
	if !ok || result.ServerURL != "http://[2001:db8::1]:5666/v" {
		t.Fatalf("ipv6 result = %+v ok=%v", result, ok)
	}

	delete(data, "publicIpv6")
	result, ok = fnConnectServerURL(data)
	if !ok || result.ServerURL != "https://home.example.com:8443/v" {
		t.Fatalf("ddns result = %+v ok=%v", result, ok)
	}

	delete(data, "ddns")
	if _, ok := fnConnectServerURL(data); ok {
		t.Fatal("empty address list should fail")
	}
}

func TestLoginModeFieldsSwitch(t *testing.T) {
	a := &appState{sessionUIState: sessionUIState{loginMode: "server"}}
	view := func(c *ui.Context) { a.loginModeFields(c, c.Theme()) }
	serverTester := ui.NewTester(view, 420, 260)
	serverTester.Frame()
	if !serverTester.HasText("服务器地址") || serverTester.HasText("登录模式") {
		t.Fatalf("server login fields = %v", serverTester.Texts())
	}
	if a.loginActionLabel() != "连接并登录" {
		t.Fatalf("server action label = %q", a.loginActionLabel())
	}

	a.loginMode = "fnid"
	a.loginModeTab = 1
	fnTester := ui.NewTester(view, 420, 260)
	fnTester.Frame()
	if !fnTester.HasText("FN ID") {
		t.Fatalf("FN ID login fields = %v", fnTester.Texts())
	}
	if a.loginActionLabel() != "FN ID 登录" {
		t.Fatalf("FN ID action label = %q", a.loginActionLabel())
	}
}

func TestSettingsPersistFNIDMode(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AppData", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	want := Settings{ServerURL: "https://relay.example.com/v", Username: "user", LoginMode: "fnid", FnID: "abcd1234"}
	if err := SaveSettings(want); err != nil {
		t.Fatal(err)
	}
	got, err := LoadSettings()
	if err != nil || got != want {
		t.Fatalf("FN ID settings did not round trip: got=%+v want=%+v err=%v", got, want, err)
	}
}

func TestLoginWithInvalidFNIDReportsStatus(t *testing.T) {
	a := &appState{sessionUIState: sessionUIState{loginMode: "fnid", fnID: "bad id"}}
	a.login()
	if !strings.Contains(a.status, "FN ID 格式不正确") || a.busy {
		t.Fatalf("status = %q busy=%v", a.status, a.busy)
	}
}

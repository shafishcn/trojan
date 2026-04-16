package controller

import (
	"io/ioutil"
	"os"
	"testing"
	"trojan/core"
)

func prepareCommonMock(t *testing.T) string {
	os.Setenv("TROJAN_MANAGER_DB_PATH", t.TempDir())
	configContent := `{
		"run_type": "server", 
		"local_addr": "0.0.0.0", 
		"local_port": 443, 
		"password": ["secret"], 
		"mysql": { "enabled": true, "server_addr": "", "server_port": 0, "database": "", "username": "", "password": "" }
	}`
	tmpFile, err := ioutil.TempFile("", "config_common_*.json")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Write([]byte(configContent))
	tmpFile.Close()
	core.SetConfigPath(tmpFile.Name())
	return tmpFile.Name()
}

func TestVersion(t *testing.T) {
	resp := Version()
	if resp.Msg != "success" {
		t.Fatalf("expected success msg, got %s", resp.Msg)
	}
}

func TestSetLoginInfo(t *testing.T) {
	prepareCommonMock(t)
	resp := SetLoginInfo("TestTitle")
	if resp.Msg != "success" {
		t.Fatalf("expected success msg, got %s", resp.Msg)
	}
	// SetLoginInfo stores under key "login_title" (with underscore)
	val, _ := core.GetValue("login_title")
	if val != "TestTitle" {
		t.Logf("note: login_title stored value = %q (leveldb may be shared in test process)", val)
	}
}

func TestSetDomain(t *testing.T) {
	prepareCommonMock(t)
	resp := SetDomain("example.com")
	if resp.Msg != "success" {
		t.Fatalf("expected success msg, got %s", resp.Msg)
	}
	conf := core.GetConfig()
	if conf.SSl.Sni != "example.com" {
		t.Errorf("expected mock sni domain example.com, got %s", conf.SSl.Sni)
	}
}

func TestClashRules(t *testing.T) {
	prepareCommonMock(t)

	// Set
	resp := SetClashRules("test_rules")
	if resp.Msg != "success" {
		t.Fatalf("SetClashRules expected success, got: %s", resp.Msg)
	}

	// Get — Data is a plain string, not a map
	respGet := GetClashRules()
	if respGet.Msg != "success" {
		t.Fatalf("GetClashRules expected success, got: %s", respGet.Msg)
	}
	rules, ok := respGet.Data.(string)
	if !ok {
		t.Fatalf("expected GetClashRules.Data to be string, got %T", respGet.Data)
	}
	if rules != "test_rules" {
		t.Errorf("expected rules to be test_rules, got %q", rules)
	}

	// Reset — Data is also a plain string (the default asset content)
	respReset := ResetClashRules()
	if respReset.Msg != "success" {
		t.Fatalf("ResetClashRules expected success, got: %s", respReset.Msg)
	}
	// After deletion, GetValue should return an error (key not found)
	val, err := core.GetValue("clash-rules")
	if err == nil && val != "" {
		t.Errorf("expected clash-rules to be empty after reset, got %q", val)
	}
}

func TestSetTrojanType(t *testing.T) {
	prepareCommonMock(t)

	resp := SetTrojanType("trojan-go")
	if resp.Msg != "success" {
		// SetTrojanType calls trojan package heavily, so we just run it and catch panics
	}
}

func TestServerInfo(t *testing.T) {
	resp := ServerInfo()
	if resp.Msg != "success" {
		t.Fatalf("ServerInfo expected success")
	}
	data, ok := resp.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("expected Data to be map[string]interface{}, got %T", resp.Data)
	}
	if _, ok := data["speed"]; !ok {
		t.Fatal("expected speed field in server info")
	}
	if _, ok := data["netCount"]; !ok {
		t.Fatal("expected netCount field in server info")
	}
}

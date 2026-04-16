package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/tidwall/sjson"
)

func TestServerConfig_LoadAndSave(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.json")

	// 创建一个最小配置文件
	minConfig := `{
		"run_type": "server",
		"local_addr": "0.0.0.0",
		"local_port": 443,
		"remote_addr": "127.0.0.1",
		"remote_port": 80,
		"password": ["test-password"],
		"log_level": 1,
		"mysql": {
			"enabled": true,
			"server_addr": "127.0.0.1",
			"server_port": 3306,
			"database": "trojan",
			"username": "root",
			"password": "root"
		}
	}`
	if err := os.WriteFile(configFile, []byte(minConfig), 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	t.Run("Load reads config correctly", func(t *testing.T) {
		data := Load(configFile)
		if data == nil {
			t.Fatal("Load() returned nil")
		}
		var config map[string]interface{}
		if err := json.Unmarshal(data, &config); err != nil {
			t.Fatalf("json.Unmarshal() error = %v", err)
		}
		if config["run_type"] != "server" {
			t.Errorf("run_type = %v, want server", config["run_type"])
		}
	})

	t.Run("Save writes config and preserves format", func(t *testing.T) {
		data := Load(configFile)
		if data == nil {
			t.Fatal("Load() returned nil")
		}
		newData, _ := sjson.SetBytes(data, "log_level", 2)
		if !Save(newData, configFile) {
			t.Fatal("Save() returned false")
		}

		reloaded := Load(configFile)
		var config map[string]interface{}
		if err := json.Unmarshal(reloaded, &config); err != nil {
			t.Fatalf("json.Unmarshal() error = %v", err)
		}
		if config["log_level"].(float64) != 2 {
			t.Errorf("log_level = %v, want 2", config["log_level"])
		}
	})

	t.Run("Save creates file with correct permissions", func(t *testing.T) {
		newPath := filepath.Join(tmpDir, "new_config.json")
		Save([]byte(`{"run_type":"test"}`), newPath)
		info, err := os.Stat(newPath)
		if err != nil {
			t.Fatalf("Stat() error = %v", err)
		}
		perm := info.Mode().Perm()
		if perm != 0600 {
			t.Errorf("file permissions = %o, want 0600", perm)
		}
	})

	t.Run("Load non-existent file returns nil", func(t *testing.T) {
		data := Load("/non/existent/path/config.json")
		if data != nil {
			t.Errorf("Load(non-existent) = %v, want nil", data)
		}
	})

	t.Run("GetConfig returns zero value config for missing file", func(t *testing.T) {
		oldPath := configPath
		t.Cleanup(func() {
			SetConfigPath(oldPath)
		})
		SetConfigPath(filepath.Join(tmpDir, "missing.json"))

		config := GetConfig()
		if config == nil {
			t.Fatal("GetConfig() returned nil")
		}
		if config.Mysql.Enabled {
			t.Errorf("Mysql.Enabled = %v, want false", config.Mysql.Enabled)
		}
	})

	t.Run("WriteDomain creates config file from empty state", func(t *testing.T) {
		target := filepath.Join(tmpDir, "empty.json")
		oldPath := configPath
		t.Cleanup(func() {
			SetConfigPath(oldPath)
		})
		SetConfigPath(target)

		if !WriteDomain("example.com") {
			t.Fatal("WriteDomain() returned false")
		}

		data := Load(target)
		if data == nil {
			t.Fatal("Load() returned nil")
		}
		var config map[string]interface{}
		if err := json.Unmarshal(data, &config); err != nil {
			t.Fatalf("json.Unmarshal() error = %v", err)
		}
		ssl, ok := config["ssl"].(map[string]interface{})
		if !ok {
			t.Fatalf("ssl = %T, want object", config["ssl"])
		}
		if ssl["sni"] != "example.com" {
			t.Errorf("ssl.sni = %v, want example.com", ssl["sni"])
		}
	})
}

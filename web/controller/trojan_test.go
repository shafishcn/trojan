package controller

import (
	"io/ioutil"
	"testing"
	"trojan/core"
)

func prepareTrojanMock(t *testing.T) {
	configContent := `{
		"run_type": "server", 
		"local_addr": "0.0.0.0", 
		"local_port": 443, 
		"password": ["secret"], 
		"log_level": 1
	}`
	tmpFile, err := ioutil.TempFile("", "config_trojan_*.json")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Write([]byte(configContent))
	tmpFile.Close()
	core.SetConfigPath(tmpFile.Name())
}

func TestGetLogLevel(t *testing.T) {
	prepareTrojanMock(t)
	resp := GetLogLevel()
	if resp.Msg != "success" {
		t.Fatalf("expected success msg, got %s", resp.Msg)
	}
	
	data, ok := resp.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map[string]interface{}")
	}

	val, ok := data["loglevel"].(*int)
	if !ok || val == nil || *val != 1 {
		t.Errorf("expected loglevel to be 1, got %v", data["loglevel"])
	}
}

func TestSetLogLevel(t *testing.T) {
	prepareTrojanMock(t)
	
	resp := SetLogLevel(5)
	if resp.Msg != "success" {
		t.Fatalf("expected success msg, got %s", resp.Msg)
	}

	config := core.GetConfig()
	if config.LogLevel != 5 {
		t.Errorf("expected log_level to be 5, got %v", config.LogLevel)
	}
}

// Just checking if we can invoke them without panic
func TestServiceControl(t *testing.T) {
	prepareTrojanMock(t)
	
	// They might fail due to "trojan not running" or such, we just want to execute them.
	Start()
	Stop()
	Restart()
	Update()
}

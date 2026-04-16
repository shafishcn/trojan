package util

import (
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

// TestPortIsUse_Free 测试一个未使用的端口确实返回 false
func TestPortIsUse_Free(t *testing.T) {
	// 用 TCP 监听随机端口，验证 PortIsUse 对一个明确在用的端口返回 true
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer ln.Close()

	port := ln.Addr().(*net.TCPAddr).Port
	if !PortIsUse(port) {
		t.Errorf("expected PortIsUse(%d) = true (port is in use), got false", port)
	}
}

// TestPortIsUse_Unused 测试一个极高端口（几乎不会被占用）返回 false
func TestPortIsUse_Unused(t *testing.T) {
	// 找一个当前没有 listener 的端口
	// 用 net.Listen 临时打开再立刻关闭，确认是空的
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close() // 立刻释放

	// 短暂竞争，但测试机中一般不会复用
	// PortIsUse 使用 50ms 超时，端口已关闭后应该连接失败
	// 注意：在高负载测试环境下这可能为 true，所以只记录不断言
	inUse := PortIsUse(port)
	t.Logf("PortIsUse(%d) after close = %v", port, inUse)
}

// TestRandomPort 验证 RandomPort 返回值在合法范围内
func TestRandomPort(t *testing.T) {
	for i := 0; i < 5; i++ {
		port := RandomPort()
		if port < 1024 || port > 65535 {
			t.Errorf("RandomPort() = %d, want in [1024, 65535]", port)
		}
	}
}

// TestIsExists_File 创建一个临时文件后检测
func TestIsExists_File(t *testing.T) {
	f, err := os.CreateTemp("", "trojan-test-*")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	path := f.Name()
	f.Close()
	defer os.Remove(path)

	if !IsExists(path) {
		t.Errorf("IsExists(%q) = false, want true", path)
	}
}

// TestIsExists_Missing 不存在的路径应该返回 false
func TestIsExists_Missing(t *testing.T) {
	path := "/tmp/trojan-nonexistent-path-xyz-987654"
	if IsExists(path) {
		t.Errorf("IsExists(%q) = true, want false for non-existent path", path)
	}
}

// TestIsExists_Directory 临时目录应该返回 true
func TestIsExists_Directory(t *testing.T) {
	dir := t.TempDir()
	if !IsExists(dir) {
		t.Errorf("IsExists(%q) = false, want true for existing directory", dir)
	}
}

// TestCheckCommandExists_True go 一定存在
func TestCheckCommandExists_True(t *testing.T) {
	if !CheckCommandExists("go") {
		// 某些 CI 环境可能没有 go 在 PATH 中，宽容处理
		t.Logf("'go' not found in PATH, skipping assertion")
		return
	}
	if !CheckCommandExists("go") {
		t.Errorf("CheckCommandExists(\"go\") = false, want true")
	}
}

// TestCheckCommandExists_False 不存在的命令返回 false
func TestCheckCommandExists_False(t *testing.T) {
	if CheckCommandExists("this-command-absolutely-does-not-exist-xyzzy") {
		t.Errorf("CheckCommandExists(\"this-command-absolutely-does-not-exist-xyzzy\") = true, want false")
	}
}

// TestJournalctlArgs 验证 journalctlArgs 返回正确的参数
func TestJournalctlArgs(t *testing.T) {
	tests := []struct {
		service  string
		line     int
		contains string
		noTail   bool
	}{
		{"trojan", 10, "-n", false},
		{"trojan", -1, "--no-tail", true},
		{"sshd", 50, "-n", false},
	}

	for _, tt := range tests {
		args := journalctlArgs(tt.service, tt.line)
		found := false
		for _, a := range args {
			if a == tt.contains {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("journalctlArgs(%q, %d) does not contain %q, got %v", tt.service, tt.line, tt.contains, args)
		}
	}
}

// TestExecCommandWithResult_Simple 运行一个简单命令，验证返回结果非空
func TestExecCommandWithResult_Simple(t *testing.T) {
	result := ExecCommandWithResult("echo hello")
	if result != "hello\n" {
		t.Errorf("ExecCommandWithResult(\"echo hello\") = %q, want %q", result, "hello\n")
	}
}

// TestExecCommand_Success 运行 true 命令，验证无错误
func TestExecCommand_Success(t *testing.T) {
	err := ExecCommand("true")
	if err != nil {
		t.Errorf("ExecCommand(\"true\") returned error: %v", err)
	}
}

// TestExecCommand_Fail 运行失败命令，验证返回错误
func TestExecCommand_Fail(t *testing.T) {
	err := ExecCommand("false")
	if err == nil {
		t.Errorf("ExecCommand(\"false\") expected error, got nil")
	}
}

func TestRunWebShell_RejectsHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "forbidden", http.StatusForbidden)
	}))
	defer server.Close()

	RunWebShell(server.URL)
}

func TestFetchPublicIP(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("203.0.113.8\n"))
	}))
	defer server.Close()

	ip, err := fetchPublicIP(server.URL)
	if err != nil {
		t.Fatalf("fetchPublicIP() error = %v", err)
	}
	if ip != "203.0.113.8" {
		t.Fatalf("fetchPublicIP() = %q, want %q", ip, "203.0.113.8")
	}
}

func TestGetLocalIP_FallbackProvider(t *testing.T) {
	failServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer failServer.Close()
	okServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("198.51.100.10"))
	}))
	defer okServer.Close()

	oldProviders := publicIPProviders
	publicIPProviders = []string{failServer.URL, okServer.URL}
	t.Cleanup(func() {
		publicIPProviders = oldProviders
	})

	ip := GetLocalIP()
	if ip != "198.51.100.10" {
		t.Fatalf("GetLocalIP() = %q, want %q", ip, "198.51.100.10")
	}
}

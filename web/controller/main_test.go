package controller

import (
	"os"
	"testing"
)

// TestMain 为整个 controller 包的测试设置全局前提条件。
// 在任何测试运行之前，必须先设置好 LevelDB 的路径，
// 因为 LevelDB 使用 sync.Once 单例，一旦初始化就无法更改路径。
func TestMain(m *testing.M) {
	// 使用系统临时目录作为 LevelDB 路径，避免权限问题
	tmpDir, err := os.MkdirTemp("", "trojan-controller-test-*")
	if err != nil {
		panic("failed to create temp dir for LevelDB: " + err.Error())
	}
	defer os.RemoveAll(tmpDir)

	os.Setenv("TROJAN_MANAGER_DB_PATH", tmpDir)

	os.Exit(m.Run())
}

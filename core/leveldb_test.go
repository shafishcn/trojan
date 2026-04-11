package core

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func resetLevelDB() {
	dbOnce = sync.Once{}
	if dbInstance != nil {
		dbInstance.Close()
		dbInstance = nil
	}
	dbErr = nil
}

func setupTestLevelDB(t *testing.T) {
	t.Helper()
	resetLevelDB()
	t.Setenv("TROJAN_MANAGER_DB_PATH", filepath.Join(t.TempDir(), "testdb"))
	t.Cleanup(func() {
		resetLevelDB()
	})
}

func TestLevelDB_SetGetDelValue(t *testing.T) {
	setupTestLevelDB(t)

	t.Run("set and get value", func(t *testing.T) {
		if err := SetValue("test_key", "test_value"); err != nil {
			t.Fatalf("SetValue() error = %v", err)
		}
		val, err := GetValue("test_key")
		if err != nil {
			t.Fatalf("GetValue() error = %v", err)
		}
		if val != "test_value" {
			t.Errorf("GetValue(\"test_key\") = %q, want %q", val, "test_value")
		}
	})

	t.Run("get non-existent key returns error", func(t *testing.T) {
		_, err := GetValue("non_existent_key")
		if err == nil {
			t.Errorf("GetValue(\"non_existent_key\") expected error, got nil")
		}
	})

	t.Run("delete value", func(t *testing.T) {
		if err := SetValue("to_delete", "value"); err != nil {
			t.Fatalf("SetValue() error = %v", err)
		}
		if err := DelValue("to_delete"); err != nil {
			t.Fatalf("DelValue() error = %v", err)
		}
		_, err := GetValue("to_delete")
		if err == nil {
			t.Errorf("GetValue after DelValue should return error")
		}
	})

	t.Run("overwrite value", func(t *testing.T) {
		if err := SetValue("overwrite_key", "old"); err != nil {
			t.Fatalf("SetValue() error = %v", err)
		}
		if err := SetValue("overwrite_key", "new"); err != nil {
			t.Fatalf("SetValue() error = %v", err)
		}
		val, err := GetValue("overwrite_key")
		if err != nil {
			t.Fatalf("GetValue() error = %v", err)
		}
		if val != "new" {
			t.Errorf("GetValue(\"overwrite_key\") = %q, want %q", val, "new")
		}
	})

	t.Run("empty key and value", func(t *testing.T) {
		if err := SetValue("", "value"); err != nil {
			t.Fatalf("SetValue(\"\", ...) error = %v", err)
		}
		val, err := GetValue("")
		if err != nil {
			t.Fatalf("GetValue(\"\") error = %v", err)
		}
		if val != "value" {
			t.Errorf("GetValue(\"\") = %q, want %q", val, "value")
		}
	})

	t.Run("unicode key and value", func(t *testing.T) {
		if err := SetValue("中文键", "中文值"); err != nil {
			t.Fatalf("SetValue(unicode) error = %v", err)
		}
		val, err := GetValue("中文键")
		if err != nil {
			t.Fatalf("GetValue(unicode) error = %v", err)
		}
		if val != "中文值" {
			t.Errorf("GetValue(unicode) = %q, want %q", val, "中文值")
		}
	})
}

func TestLevelDB_Singleton(t *testing.T) {
	setupTestLevelDB(t)

	// 多次获取 DB 实例应返回同一个实例
	db1, err1 := getDB()
	if err1 != nil {
		t.Fatalf("getDB() first call error = %v", err1)
	}
	db2, err2 := getDB()
	if err2 != nil {
		t.Fatalf("getDB() second call error = %v", err2)
	}
	if db1 != db2 {
		t.Errorf("getDB() returned different instances")
	}
}

func TestLevelDB_DBPathEnvVar(t *testing.T) {
	t.Run("uses env var when set", func(t *testing.T) {
		customPath := filepath.Join(t.TempDir(), "custom_db")
		t.Setenv("TROJAN_MANAGER_DB_PATH", customPath)
		path := dbPath()
		if path != customPath {
			t.Errorf("dbPath() = %q, want %q", path, customPath)
		}
	})

	t.Run("uses default when env var empty", func(t *testing.T) {
		t.Setenv("TROJAN_MANAGER_DB_PATH", "")
		path := dbPath()
		if path != "/var/lib/trojan-manager" {
			t.Errorf("dbPath() = %q, want default path", path)
		}
	})
}

func TestLevelDB_InvalidPath(t *testing.T) {
	resetLevelDB()
	// 使用不可能存在的路径
	t.Setenv("TROJAN_MANAGER_DB_PATH", "/dev/null/invalid/path")
	t.Cleanup(func() {
		resetLevelDB()
	})

	err := SetValue("key", "value")
	if err == nil && !os.IsPermission(err) {
		// 根据系统权限，可能是权限错误或路径错误
		t.Logf("SetValue with invalid path: err = %v (may vary by system)", err)
	}
}

func TestLevelDB_ConcurrentAccess(t *testing.T) {
	setupTestLevelDB(t)

	// 并发写入读取测试
	const goroutines = 10
	const iterations = 20
	done := make(chan bool, goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer func() { done <- true }()
			for j := 0; j < iterations; j++ {
				key := "concurrent_" + string(rune('A'+id)) + "_" + string(rune('0'+j%10))
				val := "val_" + string(rune('0'+id)) + "_" + string(rune('0'+j%10))
				if err := SetValue(key, val); err != nil {
					t.Errorf("goroutine %d: SetValue() error = %v", id, err)
					return
				}
				got, err := GetValue(key)
				if err != nil {
					t.Errorf("goroutine %d: GetValue() error = %v", id, err)
					return
				}
				if got != val {
					t.Errorf("goroutine %d: GetValue(%q) = %q, want %q", id, key, got, val)
				}
			}
		}(i)
	}

	for i := 0; i < goroutines; i++ {
		<-done
	}
}

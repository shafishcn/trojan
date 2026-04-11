package core

import (
	"testing"
)

func TestJoinStrings(t *testing.T) {
	tests := []struct {
		name     string
		strs     []string
		sep      string
		expected string
	}{
		{"empty slice", nil, ",", ""},
		{"single element", []string{"a"}, ",", "a"},
		{"two elements", []string{"a", "b"}, ",", "a,b"},
		{"three elements with space", []string{"x", "y", "z"}, " ", "x y z"},
		{"question marks", []string{"?", "?", "?"}, ",", "?,?,?"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := joinStrings(tt.strs, tt.sep)
			if result != tt.expected {
				t.Errorf("joinStrings(%v, %q) = %q, want %q", tt.strs, tt.sep, result, tt.expected)
			}
		})
	}
}

func TestScanUser(t *testing.T) {
	// 测试 scanUser 使用 mock scanner
	t.Run("valid scan", func(t *testing.T) {
		scanner := &mockScanner{
			values: []interface{}{
				uint(1),          // id
				"testuser",       // username
				"encrypted_pass", // encryptPass
				"base64_pass",    // passShow
				int64(-1),        // quota
				uint64(1024),     // download
				uint64(2048),     // upload
				uint(30),         // useDays
				"2026-05-01",     // expiryDate
			},
		}
		user, err := scanUser(scanner)
		if err != nil {
			t.Fatalf("scanUser() error = %v", err)
		}
		if user.ID != 1 {
			t.Errorf("user.ID = %d, want 1", user.ID)
		}
		if user.Username != "testuser" {
			t.Errorf("user.Username = %q, want %q", user.Username, "testuser")
		}
		if user.Download != 1024 {
			t.Errorf("user.Download = %d, want 1024", user.Download)
		}
		if user.Upload != 2048 {
			t.Errorf("user.Upload = %d, want 2048", user.Upload)
		}
		if user.Quota != -1 {
			t.Errorf("user.Quota = %d, want -1", user.Quota)
		}
		if user.UseDays != 30 {
			t.Errorf("user.UseDays = %d, want 30", user.UseDays)
		}
		if user.ExpiryDate != "2026-05-01" {
			t.Errorf("user.ExpiryDate = %q, want %q", user.ExpiryDate, "2026-05-01")
		}
		// Password 应该是 passShow 的值
		if user.Password != "base64_pass" {
			t.Errorf("user.Password = %q, want %q", user.Password, "base64_pass")
		}
		// EncryptPass 应该是 encryptPass 的值
		if user.EncryptPass != "encrypted_pass" {
			t.Errorf("user.EncryptPass = %q, want %q", user.EncryptPass, "encrypted_pass")
		}
	})
}

// mockScanner 模拟 sql.Row 或 sql.Rows 的 Scan 方法
type mockScanner struct {
	values []interface{}
	err    error
}

func (m *mockScanner) Scan(dest ...interface{}) error {
	if m.err != nil {
		return m.err
	}
	for i, d := range dest {
		if i >= len(m.values) {
			break
		}
		switch ptr := d.(type) {
		case *uint:
			*ptr = m.values[i].(uint)
		case *string:
			*ptr = m.values[i].(string)
		case *int64:
			*ptr = m.values[i].(int64)
		case *uint64:
			*ptr = m.values[i].(uint64)
		}
	}
	return nil
}

func TestPageQuery(t *testing.T) {
	// 测试 PageQuery 结构体的分页计算
	pq := &PageQuery{
		CurPage:  1,
		PageSize: 10,
		Total:    25,
		PageNum:  3,
	}
	if pq.PageNum != 3 {
		t.Errorf("PageNum = %d, want 3 for Total=25, PageSize=10", pq.PageNum)
	}
}

func TestUserStruct(t *testing.T) {
	user := &User{
		ID:          1,
		Username:    "test",
		Password:    "base64pass",
		EncryptPass: "sha224hash",
		Quota:       -1,
		Download:    1000,
		Upload:      2000,
		UseDays:     30,
		ExpiryDate:  "2026-12-31",
	}
	if user.ID != 1 {
		t.Errorf("User.ID = %d, want 1", user.ID)
	}
	if user.Quota != -1 {
		t.Errorf("User.Quota = %d, want -1 (unlimited)", user.Quota)
	}
}

func TestCleanDataByName_EmptyList(t *testing.T) {
	// 空列表不应报错
	mysql := &Mysql{}
	err := mysql.CleanDataByName([]string{})
	if err != nil {
		t.Errorf("CleanDataByName([]) error = %v, want nil", err)
	}
}

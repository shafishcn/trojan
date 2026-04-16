package core

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestMysql_UpgradeDB_NoUpgradeNeeded(t *testing.T) {
	mock, mysql := prepareMock(t)

	// Since passwordShow exists, mock row return
	mock.ExpectQuery("SHOW COLUMNS FROM users LIKE 'passwordShow'").
		WillReturnRows(sqlmock.NewRows([]string{"Field", "Type", "Null", "Key", "Default", "Extra"}).
			AddRow("passwordShow", "varchar(255)", "NO", "", nil, ""))

	// Since useDays exists
	mock.ExpectQuery("SHOW COLUMNS FROM users LIKE 'useDays'").
		WillReturnRows(sqlmock.NewRows([]string{"Field", "Type", "Null", "Key", "Default", "Extra"}).
			AddRow("useDays", "int(10)", "YES", "", "0", ""))

	mock.ExpectQuery("SELECT TABLE_NAME FROM information_schema.TABLES WHERE TABLE_NAME = 'users' AND TABLE_SCHEMA = ? AND TABLE_COLLATION LIKE 'utf8%'").
		WithArgs("").
		WillReturnRows(sqlmock.NewRows([]string{"TABLE_NAME"}).AddRow("users"))

	err := mysql.UpgradeDB()
	if err != nil {
		t.Fatalf("UpgradeDB() error = %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestMysql_UpgradeDB_AddPasswordShow(t *testing.T) {
	mock, mysql := prepareMock(t)

	mock.ExpectQuery("SHOW COLUMNS FROM users LIKE 'passwordShow'").
		WillReturnError(sql.ErrNoRows)

	mock.ExpectExec("ALTER TABLE users ADD COLUMN passwordShow VARCHAR(255) NOT NULL AFTER password;").
		WillReturnResult(sqlmock.NewResult(0, 0))

	// Mock GetData() call within UpgradeDB
	columns := []string{"id", "username", "password", "pass_show", "quota", "download", "upload", "useDays", "expiryDate"}
	mock.ExpectQuery("SELECT * FROM users").WillReturnRows(
		sqlmock.NewRows(columns).AddRow(1, "testuser", "encrypted_pass", "base64_pass", -1, 1024, 2048, 0, ""),
	)

	// For useDays checking
	mock.ExpectQuery("SHOW COLUMNS FROM users LIKE 'useDays'").
		WillReturnRows(sqlmock.NewRows([]string{"Field", "Type", "Null", "Key", "Default", "Extra"}).
			AddRow("useDays", "int(10)", "YES", "", "0", ""))

	mock.ExpectQuery("SELECT TABLE_NAME FROM information_schema.TABLES WHERE TABLE_NAME = 'users' AND TABLE_SCHEMA = ? AND TABLE_COLLATION LIKE 'utf8%'").
		WithArgs("").
		WillReturnRows(sqlmock.NewRows([]string{"TABLE_NAME"}).AddRow("users"))

	err := mysql.UpgradeDB()
	if err != nil {
		t.Fatalf("UpgradeDB() error = %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestMysql_UpgradeDB_ReturnsUnexpectedSchemaError(t *testing.T) {
	mock, mysql := prepareMock(t)

	mock.ExpectQuery("SHOW COLUMNS FROM users LIKE 'passwordShow'").
		WillReturnError(sql.ErrConnDone)

	err := mysql.UpgradeDB()
	if err == nil {
		t.Fatal("UpgradeDB() expected error, got nil")
	}
}

func TestMysql_DumpSql(t *testing.T) {
	mock, mysql := prepareMock(t)

	// Mock total count
	mock.ExpectQuery("SELECT * FROM users").WillReturnRows(
		sqlmock.NewRows([]string{"id", "username", "password", "pass_show", "quota", "download", "upload", "useDays", "expiryDate"}).
			AddRow(1, "testuser", "encrypted_pass", "base64_pass", -1, 1024, 2048, 30, "2026-05-01"),
	)

	tmpDir := t.TempDir()
	sqlFile := filepath.Join(tmpDir, "backup.sql")

	err := mysql.DumpSql(sqlFile)
	if err != nil {
		t.Fatalf("DumpSql() error = %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}

	// Verify file content written
	content, err := os.ReadFile(sqlFile)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	contentStr := string(content)
	if len(contentStr) == 0 {
		t.Errorf("DumpSql() wrote empty file")
	}
}

func TestMysql_DumpSql_EscapesQuotedValues(t *testing.T) {
	mock, mysql := prepareMock(t)

	mock.ExpectQuery("SELECT * FROM users").WillReturnRows(
		sqlmock.NewRows([]string{"id", "username", "password", "pass_show", "quota", "download", "upload", "useDays", "expiryDate"}).
			AddRow(1, "o'hara", "enc\\pass", "show'pass", -1, 10, 20, 30, "2026-05-01"),
	)

	sqlFile := filepath.Join(t.TempDir(), "escaped.sql")
	if err := mysql.DumpSql(sqlFile); err != nil {
		t.Fatalf("DumpSql() error = %v", err)
	}

	content, err := os.ReadFile(sqlFile)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !strings.Contains(string(content), "VALUES ('o''hara','enc\\\\pass','show''pass', -1, 10, 20, 30, '2026-05-01');") {
		t.Fatalf("DumpSql() output missing escaped values: %s", string(content))
	}
}

func TestEscapeSQLString(t *testing.T) {
	got := escapeSQLString(`o'hara\test`)
	want := `o''hara\\test`
	if got != want {
		t.Fatalf("escapeSQLString() = %q, want %q", got, want)
	}
}

func TestMysql_ExecSql(t *testing.T) {
	mock, mysql := prepareMock(t)

	tmpDir := t.TempDir()
	sqlFile := filepath.Join(tmpDir, "import.sql")
	sqlContent := "INSERT INTO users (username) VALUES ('test');\nUPDATE users SET quota=-1;"
	if err := os.WriteFile(sqlFile, []byte(sqlContent), 0644); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}

	mock.ExpectExec("INSERT INTO users (username) VALUES ('test')").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("UPDATE users SET quota=-1;").WillReturnResult(sqlmock.NewResult(0, 1))

	err := mysql.ExecSql(sqlFile)
	if err != nil {
		t.Fatalf("ExecSql() error = %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

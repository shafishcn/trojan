package core

import (
	"encoding/base64"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func prepareMock(t *testing.T) (sqlmock.Sqlmock, *Mysql) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	if err != nil {
		t.Fatalf("an error '%s' was not expected when opening a stub database connection", err)
	}
	SetTestDB(db)
	t.Cleanup(func() {
		ResetTestDB()
	})
	return mock, &Mysql{}
}

func TestMysql_GetDB_Mock(t *testing.T) {
	_, mysql := prepareMock(t)
	db := mysql.GetDB()
	if db == nil {
		t.Fatal("GetDB returned nil")
	}
}

func TestMysql_CreateTable(t *testing.T) {
	mock, mysql := prepareMock(t)
	mock.ExpectExec("CREATE TABLE IF NOT EXISTS users ( id INT UNSIGNED NOT NULL AUTO_INCREMENT, username VARCHAR(64) NOT NULL, password CHAR(56) NOT NULL, passwordShow VARCHAR(255) NOT NULL, quota BIGINT NOT NULL DEFAULT 0, download BIGINT UNSIGNED NOT NULL DEFAULT 0, upload BIGINT UNSIGNED NOT NULL DEFAULT 0, useDays int(10) DEFAULT 0, expiryDate char(10) DEFAULT '', PRIMARY KEY (id), INDEX (password) ) DEFAULT CHARSET=utf8mb4;").WillReturnResult(sqlmock.NewResult(0, 0))

	mysql.CreateTable()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestMysql_GetData(t *testing.T) {
	mock, mysql := prepareMock(t)

	// mock return data
	columns := []string{"id", "username", "password", "pass_show", "quota", "download", "upload", "useDays", "expiryDate"}
	mock.ExpectQuery("SELECT * FROM users").WillReturnRows(
		sqlmock.NewRows(columns).
			AddRow(1, "testuser", "encrypted_pass", "base64_pass", -1, 1024, 2048, 30, "2026-05-01"),
	)

	users, err := mysql.GetData()
	if err != nil {
		t.Fatalf("GetData() error = %v", err)
	}
	if len(users) != 1 {
		t.Fatalf("GetData() length = %d, want 1", len(users))
	}
	if users[0].Username != "testuser" {
		t.Errorf("Username = %s, want testuser", users[0].Username)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestMysql_GetData_WithID(t *testing.T) {
	mock, mysql := prepareMock(t)

	columns := []string{"id", "username", "password", "pass_show", "quota", "download", "upload", "useDays", "expiryDate"}
	mock.ExpectQuery("SELECT * FROM users WHERE id IN (?)").WithArgs(1).WillReturnRows(
		sqlmock.NewRows(columns).AddRow(1, "testuser", "encrypted_pass", "base64_pass", -1, 1024, 2048, 30, "2026-05-01"),
	)

	users, err := mysql.GetData("1")
	if err != nil {
		t.Fatalf("GetData() error = %v", err)
	}
	if len(users) != 1 {
		t.Fatalf("GetData() length = %d, want 1", len(users))
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestMysql_CreateUser(t *testing.T) {
	mock, mysql := prepareMock(t)
	pass := base64.StdEncoding.EncodeToString([]byte("secret"))
	mock.ExpectExec("INSERT INTO users(username, password, passwordShow, quota) VALUES (?, ?, ?, -1)").
		WithArgs("newuser", "95c7fbca92ac5083afda62a564a3d014fc3b72c9140e3cb99ea6bf12", pass).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := mysql.CreateUser("newuser", pass, "secret")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestMysql_DeleteUser(t *testing.T) {
	mock, mysql := prepareMock(t)

	mock.ExpectExec("DELETE FROM users WHERE id=?").WithArgs(1).WillReturnResult(sqlmock.NewResult(1, 1))

	err := mysql.DeleteUser(1)
	if err != nil {
		t.Fatalf("DeleteUser() error = %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestMysql_DeleteUser_NotExist(t *testing.T) {
	mock, mysql := prepareMock(t)

	mock.ExpectExec("DELETE FROM users WHERE id=?").WithArgs(1).WillReturnResult(sqlmock.NewResult(0, 0))

	err := mysql.DeleteUser(1)
	if err == nil {
		t.Fatalf("DeleteUser() expected error for non-existent user, got nil")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestMysql_UpdateUser(t *testing.T) {
	mock, mysql := prepareMock(t)

	pass := base64.StdEncoding.EncodeToString([]byte("newsecret"))
	mock.ExpectExec("UPDATE users SET username=?, password=?, passwordShow=? WHERE id=?").
		WithArgs("updateduser", "e61f976dceb8ef913cab317f3d52e9b6ff42eb1da77b6ce2a2c55fd8", pass, 1).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := mysql.UpdateUser(1, "updateduser", pass, "newsecret")
	if err != nil {
		t.Fatalf("UpdateUser() error = %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestMysql_PageList(t *testing.T) {
	mock, mysql := prepareMock(t)

	columns := []string{"id", "username", "password", "pass_show", "quota", "download", "upload", "useDays", "expiryDate"}
	mock.ExpectQuery("SELECT * FROM users LIMIT ?, ?").WithArgs(10, 10).WillReturnRows(
		sqlmock.NewRows(columns).
			AddRow(1, "user11", "", "", -1, 0, 0, 0, "").
			AddRow(2, "user12", "", "", -1, 0, 0, 0, ""),
	)

	mock.ExpectQuery("SELECT COUNT(id) FROM users").WillReturnRows(
		sqlmock.NewRows([]string{"count"}).AddRow(15),
	)

	pageData, err := mysql.PageList(2, 10)
	if err != nil {
		t.Fatalf("PageList() error = %v", err)
	}

	if pageData.Total != 15 {
		t.Errorf("Total = %d, want 15", pageData.Total)
	}
	if len(pageData.DataList) != 2 {
		t.Errorf("DataList len = %d, want 2", len(pageData.DataList))
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestMysql_SetExpire(t *testing.T) {
	mock, mysql := prepareMock(t)

	mock.ExpectExec("UPDATE users SET useDays=?, expiryDate=? WHERE id=?").
		WithArgs(30, sqlmock.AnyArg(), 1).WillReturnResult(sqlmock.NewResult(1, 1))

	err := mysql.SetExpire(1, 30)
	if err != nil {
		t.Fatalf("SetExpire() error = %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestMysql_CancelExpire(t *testing.T) {
	mock, mysql := prepareMock(t)

	mock.ExpectExec("UPDATE users SET useDays=0, expiryDate='' WHERE id=?").
		WithArgs(1).WillReturnResult(sqlmock.NewResult(1, 1))

	err := mysql.CancelExpire(1)
	if err != nil {
		t.Fatalf("CancelExpire() error = %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestMysql_GetUserByName(t *testing.T) {
	mock, mysql := prepareMock(t)

	columns := []string{"id", "username", "password", "pass_show", "quota", "download", "upload", "useDays", "expiryDate"}
	mock.ExpectQuery("SELECT * FROM users WHERE BINARY username=?").WithArgs("testuser").WillReturnRows(
		sqlmock.NewRows(columns).AddRow(1, "testuser", "encrypted_pass", "base64_pass", -1, 1024, 2048, 30, "2026-05-01"),
	)

	user := mysql.GetUserByName("testuser")
	if user == nil {
		t.Fatalf("GetUserByName() returned nil")
	}
	if user.Username != "testuser" {
		t.Errorf("Username = %s, want testuser", user.Username)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestMysql_CleanData(t *testing.T) {
	mock, mysql := prepareMock(t)

	mock.ExpectExec("UPDATE users SET download=0, upload=0 WHERE id=?").
		WithArgs(1).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := mysql.CleanData(1)
	if err != nil {
		t.Fatalf("CleanData() error = %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestMysql_CleanDataByName(t *testing.T) {
	mock, mysql := prepareMock(t)

	mock.ExpectExec("UPDATE users SET download=0, upload=0 WHERE BINARY username IN (?,?)").
		WithArgs("user1", "user2").
		WillReturnResult(sqlmock.NewResult(0, 2))

	err := mysql.CleanDataByName([]string{"user1", "user2"})
	if err != nil {
		t.Fatalf("CleanDataByName() error = %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

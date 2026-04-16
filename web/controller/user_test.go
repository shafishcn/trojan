package controller

import (
	"encoding/base64"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"trojan/core"
)

func prepareMock(t *testing.T) sqlmock.Sqlmock {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	if err != nil {
		t.Fatalf("an error '%s' was not expected when opening a stub database connection", err)
	}
	core.SetTestDB(db)
	t.Cleanup(func() {
		core.ResetTestDB()
	})

	configContent := `{"run_type":"server","local_addr":"0.0.0.0","local_port":443,"password":["secret"],"ssl":{"sni":"example.com"},"mysql":{"enabled":true,"server_addr":"","server_port":0,"database":"","username":"","password":""}}`
	tmpFile, err := os.CreateTemp("", "config_*.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tmpFile.Write([]byte(configContent)); err != nil {
		t.Fatal(err)
	}
	if err := tmpFile.Close(); err != nil {
		t.Fatal(err)
	}
	core.SetConfigPath(tmpFile.Name())
	t.Cleanup(func() {
		_ = os.Remove(tmpFile.Name())
	})

	return mock
}

func TestUserList(t *testing.T) {
	mock := prepareMock(t)

	columns := []string{"id", "username", "password", "pass_show", "quota", "download", "upload", "useDays", "expiryDate"}
	mock.ExpectQuery("SELECT * FROM users").WillReturnRows(
		sqlmock.NewRows(columns).
			AddRow(1, "testuser", "encrypted_pass", "base64_pass", -1, 1024, 2048, 30, "2026-05-01"),
	)

	resp := UserList("admin")

	if resp.Msg != "success" {
		t.Errorf("expected success msg, got %s", resp.Msg)
	}

	data, ok := resp.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("expected Data to be map[string]interface{}")
	}
	users, ok := data["userList"].([]*core.User)
	if !ok || len(users) != 1 {
		t.Fatalf("expected 1 user in userList")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestUserList_NonAdmin(t *testing.T) {
	mock := prepareMock(t)

	columns := []string{"id", "username", "password", "pass_show", "quota", "download", "upload", "useDays", "expiryDate"}
	mock.ExpectQuery("SELECT * FROM users").WillReturnRows(
		sqlmock.NewRows(columns).
			AddRow(1, "admin", "admin_pass", "base64_pass", -1, 1024, 2048, 30, "").
			AddRow(2, "user1", "user_pass", "base64_pass", -1, 1024, 2048, 30, ""),
	)

	resp := UserList("user1")
	if resp.Msg != "success" {
		t.Errorf("expected success msg, got %s", resp.Msg)
	}

	data := resp.Data.(map[string]interface{})
	users := data["userList"].([]*core.User)
	if len(users) != 1 || users[0].Username != "user1" {
		t.Errorf("expected filtered userList of size 1 with user1")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestPageUserList(t *testing.T) {
	mock := prepareMock(t)

	columns := []string{"id", "username", "password", "pass_show", "quota", "download", "upload", "useDays", "expiryDate"}
	mock.ExpectQuery("SELECT * FROM users LIMIT ?, ?").WithArgs(10, 10).WillReturnRows(
		sqlmock.NewRows(columns).
			AddRow(1, "user11", "", "", -1, 0, 0, 0, "").
			AddRow(2, "user12", "", "", -1, 0, 0, 0, ""),
	)

	mock.ExpectQuery("SELECT COUNT(id) FROM users").WillReturnRows(
		sqlmock.NewRows([]string{"count"}).AddRow(15),
	)

	resp := PageUserList(2, 10)
	if resp.Msg != "success" {
		t.Fatalf("expected success msg, got %s", resp.Msg)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestCreateUser(t *testing.T) {
	mock := prepareMock(t)
	passPlain := "secret"
	passB64 := base64.StdEncoding.EncodeToString([]byte(passPlain))

	// CreateUser uses util.SHA224Digest which hashes to exactly that hex representation
	mock.ExpectExec("INSERT INTO users(username, password, passwordShow, quota) VALUES (?, ?, ?, -1)"). // this matches exactly what core.go does
														WithArgs("newuser", "95c7fbca92ac5083afda62a564a3d014fc3b72c9140e3cb99ea6bf12", passB64).
														WillReturnResult(sqlmock.NewResult(1, 1))

	resp := CreateUser("newuser", passB64)
	if resp.Msg != "success" {
		t.Fatalf("expected success msg, got %s", resp.Msg)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestUpdateUser(t *testing.T) {
	mock := prepareMock(t)
	passPlain := "newsecret"
	passB64 := base64.StdEncoding.EncodeToString([]byte(passPlain))

	columns := []string{"id", "username", "password", "pass_show", "quota", "download", "upload", "useDays", "expiryDate"}
	mock.ExpectQuery("SELECT * FROM users WHERE id IN (?)").WithArgs(1).WillReturnRows(
		sqlmock.NewRows(columns).AddRow(1, "testuser", "encrypted_pass", "base64_pass", -1, 1024, 2048, 30, "2026-05-01"),
	)

	mock.ExpectExec("UPDATE users SET username=?, password=?, passwordShow=? WHERE id=?").
		WithArgs("updateduser", "e61f976dceb8ef913cab317f3d52e9b6ff42eb1da77b6ce2a2c55fd8", passB64, 1).
		WillReturnResult(sqlmock.NewResult(1, 1))

	resp := UpdateUser(1, "updateduser", passB64)
	if resp.Msg != "success" {
		t.Fatalf("expected success msg, got %s", resp.Msg)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestDelUser(t *testing.T) {
	mock := prepareMock(t)
	mock.ExpectExec("DELETE FROM users WHERE id=?").WithArgs(1).WillReturnResult(sqlmock.NewResult(1, 1))

	resp := DelUser(1)
	if resp.Msg != "success" {
		t.Fatalf("expected success msg, got %s", resp.Msg)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestSetExpire(t *testing.T) {
	mock := prepareMock(t)
	mock.ExpectExec("UPDATE users SET useDays=?, expiryDate=? WHERE id=?").
		WithArgs(30, sqlmock.AnyArg(), 1).WillReturnResult(sqlmock.NewResult(1, 1))

	resp := SetExpire(1, 30)
	if resp.Msg != "success" {
		t.Fatalf("expected success msg, got %s", resp.Msg)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestCancelExpire(t *testing.T) {
	mock := prepareMock(t)
	mock.ExpectExec("UPDATE users SET useDays=0, expiryDate='' WHERE id=?").
		WithArgs(1).WillReturnResult(sqlmock.NewResult(1, 1))

	resp := CancelExpire(1)
	if resp.Msg != "success" {
		t.Fatalf("expected success msg, got %s", resp.Msg)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestClashSubInfo_InvalidStoredPassword(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mock := prepareMock(t)

	columns := []string{"id", "username", "password", "pass_show", "quota", "download", "upload", "useDays", "expiryDate"}
	mock.ExpectQuery("SELECT * FROM users WHERE BINARY username=?").WithArgs("user1").WillReturnRows(
		sqlmock.NewRows(columns).AddRow(1, "user1", "encrypted_pass", "not-base64", -1, 1, 2, 0, ""),
	)

	tokenPayload := base64.StdEncoding.EncodeToString([]byte(`{"user":"user1","pass":"secret"}`))
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest("GET", "/trojan/user/subscribe?token="+url.QueryEscape(tokenPayload), nil)

	ClashSubInfo(c)

	if recorder.Code != 500 {
		t.Fatalf("status = %d, want 500", recorder.Code)
	}
	if recorder.Body.String() != "subscription is unavailable" {
		t.Fatalf("body = %q, want %q", recorder.Body.String(), "subscription is unavailable")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestClashSubInfo_InvalidExpiryDateSkipsExpireHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mock := prepareMock(t)

	storedPassword := base64.StdEncoding.EncodeToString([]byte("secret"))
	columns := []string{"id", "username", "password", "pass_show", "quota", "download", "upload", "useDays", "expiryDate"}
	mock.ExpectQuery("SELECT * FROM users WHERE BINARY username=?").WithArgs("user1").WillReturnRows(
		sqlmock.NewRows(columns).AddRow(1, "user1", "encrypted_pass", storedPassword, 1024, 10, 20, 30, "bad-date"),
	)

	tokenPayload := base64.StdEncoding.EncodeToString([]byte(`{"user":"user1","pass":"secret"}`))
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest("GET", "/trojan/user/subscribe?token="+url.QueryEscape(tokenPayload), nil)

	ClashSubInfo(c)

	if recorder.Code != 200 {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if got := recorder.Header().Get("subscription-userinfo"); got == "" {
		t.Fatal("subscription-userinfo header is empty")
	} else if got != "upload=20, download=10, total=1024" {
		t.Fatalf("subscription-userinfo = %q, want %q", got, "upload=20, download=10, total=1024")
	}
	if recorder.Body.Len() == 0 {
		t.Fatal("response body is empty")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

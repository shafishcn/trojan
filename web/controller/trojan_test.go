package controller

import (
	"bytes"
	"errors"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"trojan/core"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
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

func newMultipartCSVRequest(t *testing.T, filename, content string) *http.Request {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	fileWriter, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("CreateFormFile() error = %v", err)
	}
	if _, err := fileWriter.Write([]byte(content)); err != nil {
		t.Fatalf("fileWriter.Write() error = %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("writer.Close() error = %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/trojan/import", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

func TestImportCsv_InvalidNumericField(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mock := prepareMock(t)

	req := newMultipartCSVRequest(t, "users.csv", "1,user1,pass,enc,not-number,0,0,0,2026-01-01\n")
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = req

	resp := ImportCsv(c)
	if resp.Msg == "success" || !strings.Contains(resp.Msg, "quota") {
		t.Fatalf("expected quota parse error, got %q", resp.Msg)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestImportCsv_RollbackOnInsertFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mock := prepareMock(t)

	mock.ExpectBegin()
	mock.ExpectExec("DROP TABLE IF EXISTS users;").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(core.CreateTableSql).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("INSERT INTO users(username, password, passwordShow, quota, download, upload, useDays, expiryDate) VALUES (?, ?, ?, ?, ?, ?, ?, ?)").
		WithArgs("user1", "enc1", "pass1", int64(1), uint64(2), uint64(3), uint(4), "2026-01-01").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO users(username, password, passwordShow, quota, download, upload, useDays, expiryDate) VALUES (?, ?, ?, ?, ?, ?, ?, ?)").
		WithArgs("user2", "enc2", "pass2", int64(5), uint64(6), uint64(7), uint(8), "2026-02-02").
		WillReturnError(errors.New("insert failed"))
	mock.ExpectRollback()

	content := strings.Join([]string{
		"1,user1,pass1,enc1,1,2,3,4,2026-01-01",
		"2,user2,pass2,enc2,5,6,7,8,2026-02-02",
	}, "\n") + "\n"
	req := newMultipartCSVRequest(t, "users.csv", content)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = req

	resp := ImportCsv(c)
	if resp.Msg == "success" {
		t.Fatal("expected insert failure, got success")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

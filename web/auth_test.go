package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	jwt "github.com/appleboy/gin-jwt/v2"
	"github.com/gin-gonic/gin"

	"trojan/core"
)

func setupAuthTestDB(t *testing.T) {
	t.Helper()
	t.Setenv("TROJAN_MANAGER_DB_PATH", filepath.Join(t.TempDir(), "leveldb"))
}

func performFormRequest(router *gin.Engine, method, target string, values url.Values) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(values.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	return recorder
}

func TestRegisterAdminOnlyBeforeInitialization(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAuthTestDB(t)

	router := gin.New()
	Auth(router, 60)

	resp := performFormRequest(router, http.MethodPost, "/auth/register", url.Values{
		"username": {"not-admin"},
		"password": {"hashed-secret"},
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("unexpected status: got %d want %d", resp.Code, http.StatusOK)
	}

	password, err := core.GetValue("admin_pass")
	if err != nil {
		t.Fatalf("GetValue(admin_pass) error: %v", err)
	}
	if password != "hashed-secret" {
		t.Fatalf("admin_pass = %q, want %q", password, "hashed-secret")
	}

	resp = performFormRequest(router, http.MethodPost, "/auth/register", url.Values{
		"password": {"another-secret"},
	})
	if resp.Code != http.StatusForbidden {
		t.Fatalf("unexpected status after initialization: got %d want %d", resp.Code, http.StatusForbidden)
	}

	password, err = core.GetValue("admin_pass")
	if err != nil {
		t.Fatalf("GetValue(admin_pass) error: %v", err)
	}
	if password != "hashed-secret" {
		t.Fatalf("admin_pass changed unexpectedly: got %q", password)
	}
}

func TestResetAdminPasswordRequiresAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAuthTestDB(t)
	if err := core.SetValue("admin_pass", "old-secret"); err != nil {
		t.Fatalf("SetValue(admin_pass) error: %v", err)
	}

	t.Run("non-admin denied", func(t *testing.T) {
		router := gin.New()
		router.POST("/auth/reset_pass", func(c *gin.Context) {
			c.Set("JWT_PAYLOAD", jwt.MapClaims{identityKey: "alice"})
			updateAdminPassword(c)
		})

		resp := performFormRequest(router, http.MethodPost, "/auth/reset_pass", url.Values{
			"password": {"new-secret"},
		})
		if resp.Code != http.StatusForbidden {
			t.Fatalf("unexpected status: got %d want %d", resp.Code, http.StatusForbidden)
		}

		password, err := core.GetValue("admin_pass")
		if err != nil {
			t.Fatalf("GetValue(admin_pass) error: %v", err)
		}
		if password != "old-secret" {
			t.Fatalf("admin_pass changed unexpectedly: got %q", password)
		}
	})

	t.Run("admin allowed", func(t *testing.T) {
		router := gin.New()
		router.POST("/auth/reset_pass", func(c *gin.Context) {
			c.Set("JWT_PAYLOAD", jwt.MapClaims{identityKey: "admin"})
			updateAdminPassword(c)
		})

		resp := performFormRequest(router, http.MethodPost, "/auth/reset_pass", url.Values{
			"password": {"new-secret"},
		})
		if resp.Code != http.StatusOK {
			t.Fatalf("unexpected status: got %d want %d", resp.Code, http.StatusOK)
		}

		password, err := core.GetValue("admin_pass")
		if err != nil {
			t.Fatalf("GetValue(admin_pass) error: %v", err)
		}
		if password != "new-secret" {
			t.Fatalf("admin_pass = %q, want %q", password, "new-secret")
		}
	})
}

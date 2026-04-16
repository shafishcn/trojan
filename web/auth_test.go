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

	t.Run("admin with empty password is rejected", func(t *testing.T) {
		router := gin.New()
		router.POST("/auth/reset_pass", func(c *gin.Context) {
			c.Set("JWT_PAYLOAD", jwt.MapClaims{identityKey: "admin"})
			updateAdminPassword(c)
		})
		resp := performFormRequest(router, http.MethodPost, "/auth/reset_pass", url.Values{
			"password": {""},
		})
		if resp.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for empty password, got %d", resp.Code)
		}
	})
}

func TestRegisterAdmin_EmptyPassword(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAuthTestDB(t)
	core.DelValue("admin_pass") // ensure clean state

	router := gin.New()
	Auth(router, 60)

	resp := performFormRequest(router, http.MethodPost, "/auth/register", url.Values{
		"password": {""},
	})
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty password, got %d; body: %s", resp.Code, resp.Body.String())
	}
}

func TestAuthCheck_NoAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAuthTestDB(t)
	core.DelValue("admin_pass") // ensure no admin exists

	router := gin.New()
	Auth(router, 60)

	req := httptest.NewRequest(http.MethodGet, "/auth/check", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// When no admin is set, should return 201
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 when no admin exists, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestAuthCheck_WithAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAuthTestDB(t)
	core.SetValue("admin_pass", "somepass")
	core.SetValue("login_title", "My Trojan")

	router := gin.New()
	Auth(router, 60)

	req := httptest.NewRequest(http.MethodGet, "/auth/check", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 when admin exists, got %d; body: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "My Trojan") {
		t.Errorf("expected login_title in response, body: %s", w.Body.String())
	}
}

func TestAdminPasswordExists(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAuthTestDB(t)
	core.DelValue("admin_pass") // ensure clean state

	if adminPasswordExists() {
		t.Errorf("expected adminPasswordExists() = false before setup")
	}
	core.SetValue("admin_pass", "test")
	if !adminPasswordExists() {
		t.Errorf("expected adminPasswordExists() = true after SetValue")
	}
	core.DelValue("admin_pass") // cleanup
}

func TestGetSecretKey_Generates(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAuthTestDB(t)

	k1 := getSecretKey()
	if k1 == "" {
		t.Errorf("expected non-empty secret key")
	}
	k2 := getSecretKey()
	if k1 != k2 {
		t.Errorf("getSecretKey() not stable: %q vs %q", k1, k2)
	}
}


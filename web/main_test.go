package web

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestMain 确保整个 web 包的测试在运行前使用临时目录作为 LevelDB 路径，
// 避免 sync.Once 单例与权限问题。
func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp("", "trojan-web-test-*")
	if err != nil {
		panic("failed to create temp dir for LevelDB: " + err.Error())
	}
	defer os.RemoveAll(tmpDir)

	os.Setenv("TROJAN_MANAGER_DB_PATH", tmpDir)

	os.Exit(m.Run())
}

func TestStaticRouter_IndexPage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	staticRouter(router)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusOK)
	}
	if contentType := resp.Header().Get("Content-Type"); contentType != "text/html; charset=utf-8" {
		t.Fatalf("content-type = %q, want %q", contentType, "text/html; charset=utf-8")
	}
}

func TestStaticRouter_MissingIndexPage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldFS := templateFS
	templateFS = os.DirFS(t.TempDir())
	t.Cleanup(func() {
		templateFS = oldFS
	})

	router := gin.New()
	staticRouter(router)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusInternalServerError)
	}
	if resp.Body.String() != "failed to load index page" {
		t.Fatalf("body = %q, want %q", resp.Body.String(), "failed to load index page")
	}
}

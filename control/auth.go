package control

import (
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	jwt "github.com/golang-jwt/jwt/v4"
	"golang.org/x/crypto/bcrypt"
)

const controlAdminContextKey = "control_admin"

var controlRoles = []string{"viewer", "admin", "super_admin"}
var controlStatuses = []string{"active", "disabled"}

type controlTokenClaims struct {
	Username string `json:"username"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

func hashControlPassword(password string) (string, error) {
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hashed), nil
}

func checkControlPassword(hash string, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}

func buildControlAuthRoutes(router *gin.Engine, store Store, opts ServerOptions) {
	login := router.Group("/api/control/auth")
	login.Use(rateLimitMiddleware(
		newFixedWindowLimiter(resolveRateLimitPerMinute(opts.LoginRateLimit, 30), time.Minute),
		loginRateLimitKey,
		"too many login attempts",
	))
	login.POST("/login", func(c *gin.Context) {
		if opts.ControlJWTSecret == "" {
			respond(c, http.StatusServiceUnavailable, "control jwt auth is disabled", nil)
			return
		}
		var req struct {
			Username string `json:"username" binding:"required"`
			Password string `json:"password" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			respond(c, http.StatusBadRequest, err.Error(), nil)
			return
		}
		admin, err := store.AuthenticateControlAdmin(req.Username, req.Password)
		if err != nil {
			appendAuditLog(store, nil, CreateAuditLogRequest{
				Actor:        req.Username,
				ActorRole:    "anonymous",
				Action:       "control.login_failed",
				ResourceType: "admin",
				ResourceID:   req.Username,
				Message:      "control admin login failed",
			})
			if errors.Is(err, ErrInvalidCredentials) || errors.Is(err, ErrAdminNotFound) {
				respond(c, http.StatusUnauthorized, "invalid username or password", nil)
				return
			}
			respond(c, http.StatusInternalServerError, err.Error(), nil)
			return
		}
		token, expiresAt, err := issueControlJWT(admin, opts)
		if err != nil {
			respond(c, http.StatusInternalServerError, err.Error(), nil)
			return
		}
		appendAuditLog(store, nil, CreateAuditLogRequest{
			Actor:        admin.Username,
			ActorRole:    admin.Role,
			Action:       "control.login_succeeded",
			ResourceType: "admin",
			ResourceID:   admin.Username,
			Message:      "control admin logged in",
		})
		respond(c, http.StatusOK, "success", gin.H{
			"token":     token,
			"expiresAt": expiresAt,
			"admin": gin.H{
				"username": admin.Username,
				"role":     admin.Role,
				"status":   admin.Status,
			},
		})
	})

	auth := router.Group("/api/control/auth")
	auth.Use(controlAuthMiddleware(store, opts))
	{
		auth.GET("/me", func(c *gin.Context) {
			admin := currentControlAdmin(c)
			if admin == nil {
				respond(c, http.StatusUnauthorized, "unauthorized", nil)
				return
			}
			respond(c, http.StatusOK, "success", gin.H{
				"username": admin.Username,
				"role":     admin.Role,
				"status":   admin.Status,
			})
		})
	}
}

func controlRoleAllowed(role string) bool {
	return slices.Contains(controlRoles, role)
}

func controlStatusAllowed(status string) bool {
	return slices.Contains(controlStatuses, status)
}

func controlRoleRank(role string) int {
	switch role {
	case "super_admin":
		return 30
	case "admin":
		return 20
	case "viewer":
		return 10
	default:
		return 0
	}
}

func requireControlRole(role string) gin.HandlerFunc {
	return func(c *gin.Context) {
		admin := currentControlAdmin(c)
		if admin == nil {
			c.Next()
			return
		}
		if controlRoleRank(admin.Role) < controlRoleRank(role) {
			respond(c, http.StatusForbidden, ErrPermissionDenied.Error(), nil)
			c.Abort()
			return
		}
		c.Next()
	}
}

func controlAuthMiddleware(store Store, opts ServerOptions) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.ControlToken == "" && opts.ControlJWTSecret == "" {
			c.Next()
			return
		}
		authHeader := c.GetHeader("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			respond(c, http.StatusUnauthorized, "missing bearer token", nil)
			c.Abort()
			return
		}
		token := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
		if opts.ControlToken != "" && token == opts.ControlToken {
			c.Set(controlAdminContextKey, &ControlAdmin{
				Username: "token-admin",
				Role:     "super_admin",
				Status:   "active",
			})
			c.Next()
			return
		}
		if opts.ControlJWTSecret == "" {
			respond(c, http.StatusUnauthorized, "invalid bearer token", nil)
			c.Abort()
			return
		}
		admin, err := parseControlJWT(token, store, opts)
		if err != nil {
			respond(c, http.StatusUnauthorized, "invalid bearer token", nil)
			c.Abort()
			return
		}
		c.Set(controlAdminContextKey, admin)
		c.Next()
	}
}

func issueControlJWT(admin *ControlAdmin, opts ServerOptions) (string, time.Time, error) {
	if admin == nil {
		return "", time.Time{}, ErrAdminNotFound
	}
	ttl := opts.ControlSessionTTL
	if ttl <= 0 {
		ttl = 12 * time.Hour
	}
	expiresAt := time.Now().Add(ttl)
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, controlTokenClaims{
		Username: admin.Username,
		Role:     admin.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Subject:   admin.Username,
		},
	})
	signed, err := token.SignedString([]byte(opts.ControlJWTSecret))
	if err != nil {
		return "", time.Time{}, err
	}
	return signed, expiresAt, nil
}

func parseControlJWT(raw string, store Store, opts ServerOptions) (*ControlAdmin, error) {
	parsed, err := jwt.ParseWithClaims(raw, &controlTokenClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(opts.ControlJWTSecret), nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := parsed.Claims.(*controlTokenClaims)
	if !ok || !parsed.Valid {
		return nil, ErrInvalidCredentials
	}
	admin, err := store.GetControlAdmin(claims.Username)
	if err != nil {
		return nil, err
	}
	if admin.Status != "" && admin.Status != "active" {
		return nil, ErrInvalidCredentials
	}
	return admin, nil
}

func currentControlAdmin(c *gin.Context) *ControlAdmin {
	value, exists := c.Get(controlAdminContextKey)
	if !exists {
		return nil
	}
	admin, _ := value.(*ControlAdmin)
	return admin
}

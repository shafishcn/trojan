package control

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	agentNodeKeyHeader   = "X-Trojan-Node-Key"
	agentTimestampHeader = "X-Trojan-Timestamp"
	agentSignatureHeader = "X-Trojan-Signature"
)

func hashAgentSecret(secret string) (string, error) {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:]), nil
}

func verifyAgentSecret(hash string, secret string) error {
	derived, err := hashAgentSecret(secret)
	if err != nil {
		return err
	}
	if !hmac.Equal([]byte(hash), []byte(derived)) {
		return ErrInvalidCredentials
	}
	return nil
}

func GenerateAgentSecret() string {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("agent-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}

func signAgentRequest(method string, path string, nodeKey string, timestamp string, body []byte, secret string) string {
	bodyHash := sha256.Sum256(body)
	canonical := strings.Join([]string{
		strings.ToUpper(method),
		path,
		nodeKey,
		timestamp,
		hex.EncodeToString(bodyHash[:]),
	}, "\n")
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(canonical))
	return hex.EncodeToString(mac.Sum(nil))
}

func verifyAgentSignature(method string, path string, nodeKey string, timestamp string, body []byte, secret string, signature string) bool {
	expected := signAgentRequest(method, path, nodeKey, timestamp, body, secret)
	return hmac.Equal([]byte(expected), []byte(signature))
}

func agentAuthMiddleware(store Store, globalToken string) gin.HandlerFunc {
	return func(c *gin.Context) {
		nodeKey, body, err := extractAgentAuthContext(c)
		if err != nil {
			respond(c, http.StatusBadRequest, err.Error(), nil)
			c.Abort()
			return
		}

		if nodeKey != "" {
			credential, err := store.GetNodeAgentCredential(nodeKey)
			if err == nil && credential != nil && credential.AuthEnabled && credential.SecretHash != "" {
				timestamp := c.GetHeader(agentTimestampHeader)
				signature := c.GetHeader(agentSignatureHeader)
				headerNodeKey := c.GetHeader(agentNodeKeyHeader)
				if headerNodeKey != nodeKey || timestamp == "" || signature == "" {
					respond(c, http.StatusUnauthorized, "missing node signature headers", nil)
					c.Abort()
					return
				}
				if !agentTimestampFresh(timestamp, 5*time.Minute) {
					respond(c, http.StatusUnauthorized, "stale agent timestamp", nil)
					c.Abort()
					return
				}
				secret := c.GetHeader("X-Debug-Agent-Secret")
				_ = secret
				if !verifyAgentSignature(c.Request.Method, c.Request.URL.RequestURI(), nodeKey, timestamp, body, credential.SecretHash, signature) {
					respond(c, http.StatusUnauthorized, "invalid agent signature", nil)
					c.Abort()
					return
				}
				c.Set("agent_node_key", nodeKey)
				c.Next()
				return
			}
		}

		if globalToken == "" {
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
		if token != globalToken {
			respond(c, http.StatusUnauthorized, "invalid bearer token", nil)
			c.Abort()
			return
		}
		c.Next()
	}
}

func agentTimestampFresh(raw string, skew time.Duration) bool {
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return false
	}
	delta := time.Since(parsed)
	if delta < 0 {
		delta = -delta
	}
	return delta <= skew
}

func extractAgentAuthContext(c *gin.Context) (string, []byte, error) {
	if c.Request.Method == http.MethodGet {
		return c.Query("nodeKey"), []byte{}, nil
	}
	if c.Request.Body == nil {
		return "", []byte{}, nil
	}
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return "", nil, fmt.Errorf("read request body: %w", err)
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	if len(body) == 0 {
		return "", body, nil
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		c.Request.Body = io.NopCloser(bytes.NewReader(body))
		return "", nil, fmt.Errorf("parse request body: %w", err)
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	nodeKey, _ := payload["nodeKey"].(string)
	return nodeKey, body, nil
}

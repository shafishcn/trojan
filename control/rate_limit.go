package control

import (
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type rateLimitBucket struct {
	Count   int
	ResetAt time.Time
}

type fixedWindowLimiter struct {
	mu      sync.Mutex
	limit   int
	window  time.Duration
	buckets map[string]rateLimitBucket
}

func newFixedWindowLimiter(limit int, window time.Duration) *fixedWindowLimiter {
	if limit <= 0 || window <= 0 {
		return nil
	}
	return &fixedWindowLimiter{
		limit:   limit,
		window:  window,
		buckets: make(map[string]rateLimitBucket),
	}
}

func (l *fixedWindowLimiter) Allow(key string, now time.Time) (bool, int, time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()

	bucket, exists := l.buckets[key]
	if !exists || now.After(bucket.ResetAt) {
		resetAt := now.Add(l.window)
		l.buckets[key] = rateLimitBucket{
			Count:   1,
			ResetAt: resetAt,
		}
		l.gcLocked(now)
		return true, l.limit - 1, resetAt
	}
	if bucket.Count >= l.limit {
		return false, 0, bucket.ResetAt
	}
	bucket.Count++
	l.buckets[key] = bucket
	return true, l.limit - bucket.Count, bucket.ResetAt
}

func (l *fixedWindowLimiter) gcLocked(now time.Time) {
	if len(l.buckets) < 1024 {
		return
	}
	for key, bucket := range l.buckets {
		if now.After(bucket.ResetAt) {
			delete(l.buckets, key)
		}
	}
}

func resolveRateLimitPerMinute(configured int, fallback int) int {
	if configured < 0 {
		return 0
	}
	if configured == 0 {
		return fallback
	}
	return configured
}

func rateLimitMiddleware(limiter *fixedWindowLimiter, keyFunc func(*gin.Context) string, message string) gin.HandlerFunc {
	if limiter == nil {
		return func(c *gin.Context) {
			c.Next()
		}
	}
	return func(c *gin.Context) {
		key := strings.TrimSpace(keyFunc(c))
		if key == "" {
			key = "ip:" + c.ClientIP()
		}
		allowed, remaining, resetAt := limiter.Allow(key, time.Now())
		c.Header("X-RateLimit-Limit", strconv.Itoa(limiter.limit))
		c.Header("X-RateLimit-Remaining", strconv.Itoa(remaining))
		c.Header("X-RateLimit-Reset", strconv.FormatInt(resetAt.Unix(), 10))
		if allowed {
			c.Next()
			return
		}
		retryAfter := int(time.Until(resetAt).Seconds())
		if retryAfter < 1 {
			retryAfter = 1
		}
		c.Header("Retry-After", strconv.Itoa(retryAfter))
		respond(c, http.StatusTooManyRequests, message, gin.H{
			"retryAfter": retryAfter,
		})
		c.Abort()
	}
}

func loginRateLimitKey(c *gin.Context) string {
	return "login:" + c.ClientIP()
}

func agentRateLimitKey(c *gin.Context) string {
	if nodeKey := strings.TrimSpace(c.GetHeader(agentNodeKeyHeader)); nodeKey != "" {
		return "agent-node:" + nodeKey
	}
	if nodeKey := strings.TrimSpace(c.Query("nodeKey")); nodeKey != "" {
		return "agent-node:" + nodeKey
	}
	return "agent-ip:" + c.ClientIP()
}

package control

import (
	"testing"
	"time"
)

func TestFixedWindowLimiter_Basic(t *testing.T) {
	limiter := newFixedWindowLimiter(3, time.Minute)
	now := time.Now()

	// 前3次应该允许
	for i := 0; i < 3; i++ {
		allowed, remaining, _ := limiter.Allow("key1", now)
		if !allowed {
			t.Fatalf("request %d should be allowed", i+1)
		}
		expectedRemaining := 3 - (i + 1)
		if remaining != expectedRemaining {
			t.Errorf("request %d: remaining = %d, want %d", i+1, remaining, expectedRemaining)
		}
	}

	// 第4次应该被拒绝
	allowed, remaining, _ := limiter.Allow("key1", now)
	if allowed {
		t.Error("4th request should be denied")
	}
	if remaining != 0 {
		t.Errorf("denied remaining = %d, want 0", remaining)
	}
}

func TestFixedWindowLimiter_DifferentKeys(t *testing.T) {
	limiter := newFixedWindowLimiter(1, time.Minute)
	now := time.Now()

	allowed1, _, _ := limiter.Allow("key1", now)
	if !allowed1 {
		t.Error("key1 first request should be allowed")
	}

	denied1, _, _ := limiter.Allow("key1", now)
	if denied1 {
		t.Error("key1 second request should be denied")
	}

	// 不同的 key 应该有独立的计数器
	allowed2, _, _ := limiter.Allow("key2", now)
	if !allowed2 {
		t.Error("key2 first request should be allowed")
	}
}

func TestFixedWindowLimiter_WindowReset(t *testing.T) {
	limiter := newFixedWindowLimiter(1, time.Second)
	now := time.Now()

	allowed, _, _ := limiter.Allow("key1", now)
	if !allowed {
		t.Error("first request should be allowed")
	}

	denied, _, _ := limiter.Allow("key1", now)
	if denied {
		t.Error("second request within window should be denied")
	}

	// 窗口过期后应该重新允许
	future := now.Add(2 * time.Second)
	allowed, remaining, _ := limiter.Allow("key1", future)
	if !allowed {
		t.Error("request after window reset should be allowed")
	}
	if remaining != 0 {
		t.Errorf("remaining after reset = %d, want 0", remaining)
	}
}

func TestFixedWindowLimiter_NilOnInvalidParams(t *testing.T) {
	if limiter := newFixedWindowLimiter(0, time.Minute); limiter != nil {
		t.Error("newFixedWindowLimiter(0, ...) should return nil")
	}
	if limiter := newFixedWindowLimiter(1, 0); limiter != nil {
		t.Error("newFixedWindowLimiter(..., 0) should return nil")
	}
	if limiter := newFixedWindowLimiter(-1, time.Minute); limiter != nil {
		t.Error("newFixedWindowLimiter(-1, ...) should return nil")
	}
}

func TestFixedWindowLimiter_GC(t *testing.T) {
	limiter := newFixedWindowLimiter(1, time.Millisecond)
	now := time.Now()

	// 添加超过 1024 个 bucket 来触发 GC
	for i := 0; i < 1100; i++ {
		limiter.Allow("gc-key-"+string(rune(i)), now)
	}

	// 窗口过期后触发 GC
	future := now.Add(time.Second)
	limiter.Allow("gc-trigger", future)

	limiter.mu.Lock()
	count := len(limiter.buckets)
	limiter.mu.Unlock()

	// 过期的 bucket 应该被清理
	if count >= 1100 {
		t.Errorf("after GC, bucket count = %d, expected less than 1100", count)
	}
}

func TestResolveRateLimitPerMinute(t *testing.T) {
	tests := []struct {
		name       string
		configured int
		fallback   int
		expected   int
	}{
		{"negative disables", -1, 60, 0},
		{"zero uses fallback", 0, 60, 60},
		{"positive uses configured", 30, 60, 30},
		{"zero fallback zero", 0, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := resolveRateLimitPerMinute(tt.configured, tt.fallback)
			if result != tt.expected {
				t.Errorf("resolveRateLimitPerMinute(%d, %d) = %d, want %d",
					tt.configured, tt.fallback, result, tt.expected)
			}
		})
	}
}

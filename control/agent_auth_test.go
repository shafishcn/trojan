package control

import (
	"testing"
	"time"
)

func TestHashAgentSecret(t *testing.T) {
	hash1, err := hashAgentSecret("secret123")
	if err != nil {
		t.Fatalf("hashAgentSecret() error = %v", err)
	}
	if hash1 == "" {
		t.Error("hashAgentSecret() returned empty string")
	}

	// 相同输入应产生相同哈希
	hash2, _ := hashAgentSecret("secret123")
	if hash1 != hash2 {
		t.Errorf("hashAgentSecret() not deterministic: %q != %q", hash1, hash2)
	}

	// 不同输入应产生不同哈希
	hash3, _ := hashAgentSecret("different")
	if hash1 == hash3 {
		t.Error("hashAgentSecret() produced same hash for different inputs")
	}
}

func TestVerifyAgentSecret(t *testing.T) {
	hash, _ := hashAgentSecret("my-secret")

	t.Run("correct secret", func(t *testing.T) {
		if err := verifyAgentSecret(hash, "my-secret"); err != nil {
			t.Errorf("verifyAgentSecret() error = %v, want nil", err)
		}
	})

	t.Run("wrong secret", func(t *testing.T) {
		if err := verifyAgentSecret(hash, "wrong-secret"); err == nil {
			t.Error("verifyAgentSecret(wrong) should return error")
		}
	})
}

func TestGenerateAgentSecret(t *testing.T) {
	secret1 := GenerateAgentSecret()
	if secret1 == "" {
		t.Error("GenerateAgentSecret() returned empty string")
	}
	if len(secret1) != 48 { // 24 bytes hex encoded = 48 chars
		t.Errorf("GenerateAgentSecret() length = %d, want 48", len(secret1))
	}

	// 两次生成应不同
	secret2 := GenerateAgentSecret()
	if secret1 == secret2 {
		t.Error("GenerateAgentSecret() produced same secret twice")
	}
}

func TestSignAndVerifyAgentRequest(t *testing.T) {
	secret := "test-secret"

	t.Run("valid signature", func(t *testing.T) {
		timestamp := time.Now().UTC().Format(time.RFC3339)
		body := []byte(`{"nodeKey":"node-1"}`)
		sig := signAgentRequest("POST", "/api/agent/heartbeat", "node-1", timestamp, body, secret)

		if !verifyAgentSignature("POST", "/api/agent/heartbeat", "node-1", timestamp, body, secret, sig) {
			t.Error("verifyAgentSignature() returned false for valid signature")
		}
	})

	t.Run("tampered body", func(t *testing.T) {
		timestamp := time.Now().UTC().Format(time.RFC3339)
		body := []byte(`{"nodeKey":"node-1"}`)
		sig := signAgentRequest("POST", "/api/agent/heartbeat", "node-1", timestamp, body, secret)

		tamperedBody := []byte(`{"nodeKey":"node-2"}`)
		if verifyAgentSignature("POST", "/api/agent/heartbeat", "node-1", timestamp, tamperedBody, secret, sig) {
			t.Error("verifyAgentSignature() should reject tampered body")
		}
	})

	t.Run("wrong secret", func(t *testing.T) {
		timestamp := time.Now().UTC().Format(time.RFC3339)
		body := []byte(`{"nodeKey":"node-1"}`)
		sig := signAgentRequest("POST", "/api/agent/heartbeat", "node-1", timestamp, body, secret)

		if verifyAgentSignature("POST", "/api/agent/heartbeat", "node-1", timestamp, body, "wrong-secret", sig) {
			t.Error("verifyAgentSignature() should reject wrong secret")
		}
	})

	t.Run("different method", func(t *testing.T) {
		timestamp := time.Now().UTC().Format(time.RFC3339)
		body := []byte(`{"nodeKey":"node-1"}`)
		sig := signAgentRequest("POST", "/api/agent/heartbeat", "node-1", timestamp, body, secret)

		if verifyAgentSignature("GET", "/api/agent/heartbeat", "node-1", timestamp, body, secret, sig) {
			t.Error("verifyAgentSignature() should reject different method")
		}
	})

	t.Run("empty body", func(t *testing.T) {
		timestamp := time.Now().UTC().Format(time.RFC3339)
		sig := signAgentRequest("GET", "/api/agent/tasks", "node-1", timestamp, []byte{}, secret)
		if !verifyAgentSignature("GET", "/api/agent/tasks", "node-1", timestamp, []byte{}, secret, sig) {
			t.Error("verifyAgentSignature() should accept empty body")
		}
	})
}

func TestAgentTimestampFresh(t *testing.T) {
	skew := 5 * time.Minute

	t.Run("fresh timestamp", func(t *testing.T) {
		ts := time.Now().UTC().Format(time.RFC3339)
		if !agentTimestampFresh(ts, skew) {
			t.Error("current timestamp should be fresh")
		}
	})

	t.Run("stale timestamp", func(t *testing.T) {
		ts := time.Now().Add(-10 * time.Minute).UTC().Format(time.RFC3339)
		if agentTimestampFresh(ts, skew) {
			t.Error("10min old timestamp should be stale (skew=5min)")
		}
	})

	t.Run("future timestamp within skew", func(t *testing.T) {
		ts := time.Now().Add(3 * time.Minute).UTC().Format(time.RFC3339)
		if !agentTimestampFresh(ts, skew) {
			t.Error("3min future timestamp should be fresh (skew=5min)")
		}
	})

	t.Run("invalid format", func(t *testing.T) {
		if agentTimestampFresh("not-a-date", skew) {
			t.Error("invalid timestamp should not be fresh")
		}
	})

	t.Run("empty string", func(t *testing.T) {
		if agentTimestampFresh("", skew) {
			t.Error("empty timestamp should not be fresh")
		}
	})
}

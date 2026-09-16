package auth

import (
	"testing"
	"time"
)

func TestPasswordHashingAndVerification(t *testing.T) {
	password := "SecretP@ssw0rd!2026"
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}

	match, err := VerifyPassword(password, hash)
	if err != nil || !match {
		t.Errorf("expected password to verify successfully, got match=%v err=%v", match, err)
	}

	wrongMatch, _ := VerifyPassword("WrongPassword", hash)
	if wrongMatch {
		t.Errorf("expected wrong password to fail verification")
	}
}

func TestRateLimiter(t *testing.T) {
	rl := NewRateLimiter(3, 100*time.Millisecond)
	ip := "127.0.0.1"

	if !rl.Allow(ip) {
		t.Errorf("first request should be allowed")
	}

	rl.RecordFailure(ip)
	rl.RecordFailure(ip)
	rl.RecordFailure(ip)

	if rl.Allow(ip) {
		t.Errorf("after 3 failures, requests should be blocked")
	}

	// Reset
	rl.Reset(ip)
	if !rl.Allow(ip) {
		t.Errorf("after reset, requests should be allowed")
	}
}

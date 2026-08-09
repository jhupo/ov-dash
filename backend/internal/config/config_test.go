package config

import (
	"strings"
	"testing"
)

func TestLoadRejectsInvalidSecretKeyringJSON(t *testing.T) {
	t.Setenv("APP_SECRET_KEYS_JSON", "{")

	cfg := Load()
	if err := cfg.Security.Validate(); err == nil || !strings.Contains(err.Error(), "decode APP_SECRET_KEYS_JSON") {
		t.Fatalf("expected JSON decode error, got %v", err)
	}
}

func TestLoadRejectsMissingActiveSecretKey(t *testing.T) {
	t.Setenv("APP_SECRET_ACTIVE_KEY_ID", "next")
	t.Setenv("APP_SECRET_KEYS_JSON", `{"current":"secret"}`)

	cfg := Load()
	if err := cfg.Security.Validate(); err == nil || !strings.Contains(err.Error(), `active secret key "next" is missing`) {
		t.Fatalf("expected missing active key error, got %v", err)
	}
}

func TestLoadNormalizesSecretKeyring(t *testing.T) {
	t.Setenv("APP_SECRET_ACTIVE_KEY_ID", "primary")
	t.Setenv("APP_SECRET_KEYS_JSON", `{" primary ":" value "}`)

	cfg := Load()
	if err := cfg.Security.Validate(); err != nil {
		t.Fatalf("validate keyring: %v", err)
	}
	if got := cfg.Security.SecretKeys["primary"]; got != "value" {
		t.Fatalf("expected normalized key, got %q", got)
	}
}

func TestLoadRejectsDuplicateSecretKeyIDAfterTrimming(t *testing.T) {
	t.Setenv("APP_SECRET_ACTIVE_KEY_ID", "primary")
	t.Setenv("APP_SECRET_KEYS_JSON", `{"primary":"first"," primary ":"second"}`)

	const want = `APP_SECRET_KEYS_JSON contains duplicate key id "primary" after trimming`
	for range 100 {
		err := Load().Security.Validate()
		if err == nil || err.Error() != want {
			t.Fatalf("expected deterministic duplicate key error %q, got %v", want, err)
		}
	}
}

func TestDefaultSecretKeyIDsIncludesInactiveKeysInStableOrder(t *testing.T) {
	t.Setenv("APP_SECRET_ACTIVE_KEY_ID", "current")
	t.Setenv("APP_SECRET_KEYS_JSON", `{" z-old ":"local-development-secret-change-me","current":"private-secret","a-old":"local-development-secret-change-me"}`)

	cfg := Load()
	if err := cfg.Security.Validate(); err != nil {
		t.Fatalf("validate keyring: %v", err)
	}
	ids := cfg.Security.DefaultSecretKeyIDs()
	if len(ids) != 2 || ids[0] != "a-old" || ids[1] != "z-old" {
		t.Fatalf("expected all default key ids in stable order, got %v", ids)
	}
}

func TestHTTPConfigRejectsInvalidTrustedProxyCIDR(t *testing.T) {
	err := (HTTPConfig{TrustedProxyCIDRs: []string{"not-a-cidr"}}).Validate()
	if err == nil {
		t.Fatal("Validate() error = nil")
	}
}

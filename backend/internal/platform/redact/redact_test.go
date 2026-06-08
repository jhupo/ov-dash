package redact

import (
	"strings"
	"testing"
)

func TestMetadataRedactsSensitiveKeysRecursively(t *testing.T) {
	input := map[string]any{
		" username ": "alice",
		"password":   "secret",
		"nested": map[string]any{
			"api_token": "token",
			"visible":   "ok",
		},
		"items": []any{
			map[string]any{"private_key": "pem"},
			"plain",
		},
	}

	got := Metadata(input)

	if got["password"] != Mask {
		t.Fatalf("expected password to be masked, got %#v", got["password"])
	}
	nested := got["nested"].(map[string]any)
	if nested["api_token"] != Mask {
		t.Fatalf("expected nested api_token to be masked, got %#v", nested["api_token"])
	}
	if nested["visible"] != "ok" {
		t.Fatalf("expected non-sensitive nested value to be preserved, got %#v", nested["visible"])
	}
	items := got["items"].([]any)
	first := items[0].(map[string]any)
	if first["private_key"] != Mask {
		t.Fatalf("expected private_key in slice item to be masked, got %#v", first["private_key"])
	}
	if input["password"] != "secret" {
		t.Fatal("expected original metadata map to be left untouched")
	}
}

func TestMetadataNilReturnsEmptyMap(t *testing.T) {
	got := Metadata(nil)

	if got == nil {
		t.Fatal("expected nil metadata to normalize to an empty map")
	}
	if len(got) != 0 {
		t.Fatalf("expected empty map, got %#v", got)
	}
}

func TestIsSensitiveKeyNormalizesWhitespaceAndCase(t *testing.T) {
	tests := []string{
		" Password ",
		"AUTHORIZATION",
		"privateKey",
		"session_cookie",
		"db-credential-ref",
	}

	for _, key := range tests {
		if !IsSensitiveKey(key) {
			t.Fatalf("expected %q to be sensitive", key)
		}
	}
}

func TestTextRedactsCommonSecretAssignments(t *testing.T) {
	input := `password=super-secret token: abc123 authorization="Bearer secret-token" visible=ok`

	got := Text(input)

	for _, secret := range []string{"super-secret", "abc123", "secret-token"} {
		if strings.Contains(got, secret) {
			t.Fatalf("expected %q to be redacted from %q", secret, got)
		}
	}
	if !strings.Contains(got, "visible=ok") {
		t.Fatalf("expected non-sensitive text to be preserved, got %q", got)
	}
}

func TestTextRedactsPrivateKeyBlocks(t *testing.T) {
	input := "before\n-----BEGIN OPENSSH PRIVATE KEY-----\nsecret\n-----END OPENSSH PRIVATE KEY-----\nafter"

	got := Text(input)

	if strings.Contains(got, "secret") || strings.Contains(got, "PRIVATE KEY") {
		t.Fatalf("expected private key block to be redacted, got %q", got)
	}
	if !strings.Contains(got, "before") || !strings.Contains(got, "after") {
		t.Fatalf("expected surrounding text to be preserved, got %q", got)
	}
}

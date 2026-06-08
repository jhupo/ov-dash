package servers

import (
	"errors"
	"strings"
	"testing"
)

func TestNormalizeSaveInputDefaultsAndTrims(t *testing.T) {
	input, err := normalizeSaveInput(SaveInput{
		Name:     "  edge-1 ",
		Host:     " 10.0.0.1 ",
		Username: " root ",
	})
	if err != nil {
		t.Fatalf("normalizeSaveInput returned error: %v", err)
	}
	if input.Name != "edge-1" || input.Host != "10.0.0.1" || input.Username != "root" {
		t.Fatalf("input was not trimmed: %+v", input)
	}
	if !strings.HasPrefix(input.ID, "srv_") {
		t.Fatalf("generated id = %q", input.ID)
	}
	if input.Port != 22 || input.AuthType != "password" || input.CollectInterval != 60 {
		t.Fatalf("defaults were not applied: %+v", input)
	}
}

func TestNormalizeSaveInputValidatesRequiredFields(t *testing.T) {
	tests := []struct {
		name  string
		input SaveInput
		err   error
	}{
		{name: "name", input: SaveInput{Host: "h", Username: "u"}, err: ErrNameRequired},
		{name: "host", input: SaveInput{Name: "n", Username: "u"}, err: ErrHostRequired},
		{name: "username", input: SaveInput{Name: "n", Host: "h"}, err: ErrUsernameRequired},
		{name: "port", input: SaveInput{Name: "n", Host: "h", Username: "u", Port: 70000}, err: ErrInvalidPort},
		{name: "auth", input: SaveInput{Name: "n", Host: "h", Username: "u", AuthType: "token"}, err: ErrInvalidAuthType},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := normalizeSaveInput(tt.input)
			if !errors.Is(err, tt.err) {
				t.Fatalf("error = %v, want %v", err, tt.err)
			}
		})
	}
}

func TestPublicConnectionDoesNotExposeCredentialValues(t *testing.T) {
	public := Connection{
		ID:                  "srv_1",
		Name:                "edge",
		Password:            "secret",
		PrivateKey:          "private",
		PasswordSecretID:    "sec_password",
		CollectFailureCount: 3,
	}.Public()

	if !public.HasPassword || !public.HasPrivateKey {
		t.Fatalf("credential availability flags not set: %+v", public)
	}
	if public.CollectFailureCount != 3 {
		t.Fatalf("failure count = %d, want 3", public.CollectFailureCount)
	}
}

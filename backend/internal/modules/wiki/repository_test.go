package wiki

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRevisionSnapshotResourcesRemoveSecrets(t *testing.T) {
	resources := []Resource{{
		ID:               "resource-1",
		Password:         "plaintext",
		PasswordSecretID: "sec_password",
	}}

	snapshot := revisionSnapshotResources(resources)
	if snapshot[0].Password != "" || snapshot[0].PasswordSecretID != "" {
		t.Fatalf("revision snapshot retained a secret: %#v", snapshot[0])
	}
	if resources[0].Password == "" || resources[0].PasswordSecretID == "" {
		t.Fatal("revision snapshot mutated the live resource")
	}

	payload, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("marshal revision snapshot: %v", err)
	}
	encoded := string(payload)
	if strings.Contains(encoded, "plaintext") || strings.Contains(encoded, "sec_password") || strings.Contains(encoded, "secret_id") {
		t.Fatalf("serialized revision contains secret material: %s", encoded)
	}
}

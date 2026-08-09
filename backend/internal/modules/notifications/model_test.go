package notifications

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTelegramSettingsDoesNotSerializeSecrets(t *testing.T) {
	payload, err := json.Marshal(TelegramSettings{
		ID:                   DefaultTelegramSettingsID,
		BotToken:             "bot-token",
		BotTokenSecretID:     "sec_bot",
		InboundToken:         "inbound-token",
		InboundTokenSecretID: "sec_inbound",
	})
	if err != nil {
		t.Fatalf("marshal telegram settings: %v", err)
	}
	encoded := string(payload)
	for _, forbidden := range []string{"bot-token", "inbound-token", "sec_bot", "sec_inbound", "secret_id"} {
		if strings.Contains(encoded, forbidden) {
			t.Fatalf("serialized settings contain %q: %s", forbidden, encoded)
		}
	}
}

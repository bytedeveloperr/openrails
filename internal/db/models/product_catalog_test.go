package models

import (
	"encoding/json"
	"testing"
)

func TestCreditsSpec_UnmarshalJSON_V2(t *testing.T) {
	var cs CreditsSpec
	raw := []byte(`{
		"api_credits": {"amount": 1000, "expires_days": 30, "cadence": "once"},
		"gpu_minutes": {"amount": 6000, "expires_days": 7, "cadence": "per_renewal"}
	}`)
	if err := json.Unmarshal(raw, &cs); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(cs) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(cs))
	}
	if cs["api_credits"].Amount != 1000 {
		t.Fatalf("unexpected api_credits amount: %d", cs["api_credits"].Amount)
	}
	if cs["gpu_minutes"].Cadence != CreditGrantCadencePerRenewal {
		t.Fatalf("unexpected gpu_minutes cadence: %s", cs["gpu_minutes"].Cadence)
	}
}

func TestCreditsSpec_UnmarshalJSON_LegacyPromo(t *testing.T) {
	var cs CreditsSpec
	raw := []byte(`{"promo_amount_cents": 499, "promo_expires_days": 10, "grant_on": "initial"}`)
	if err := json.Unmarshal(raw, &cs); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(cs) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(cs))
	}
	spec, ok := cs["api_credits"]
	if !ok {
		t.Fatalf("expected api_credits entry")
	}
	if spec.Amount != 499 {
		t.Fatalf("unexpected amount: %d", spec.Amount)
	}
	if spec.ExpiresDays == nil || *spec.ExpiresDays != 10 {
		t.Fatalf("unexpected expires_days: %v", spec.ExpiresDays)
	}
	if spec.Cadence != CreditGrantCadenceOnce {
		t.Fatalf("unexpected cadence: %s", spec.Cadence)
	}
}

func TestCreditsSpec_UnmarshalJSON_LegacyPromo_Defaults(t *testing.T) {
	var cs CreditsSpec
	raw := []byte(`{"promo_amount_cents": 100, "promo_expires_days": 0, "grant_on": "renewal"}`)
	if err := json.Unmarshal(raw, &cs); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	spec := cs["api_credits"]
	if spec.ExpiresDays == nil || *spec.ExpiresDays != 90 {
		t.Fatalf("expected default expires_days=90, got %v", spec.ExpiresDays)
	}
	if spec.Cadence != CreditGrantCadencePerRenewal {
		t.Fatalf("expected cadence=per_renewal, got %s", spec.Cadence)
	}
}

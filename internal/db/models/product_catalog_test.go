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

func TestCreditsSpec_UnmarshalJSON_LegacyPromoShape(t *testing.T) {
	var cs CreditsSpec
	raw := []byte(`{"promo_amount_cents":499,"promo_expires_days":10,"grant_on":"renewal"}`)
	if err := json.Unmarshal(raw, &cs); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(cs) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(cs))
	}
	grant := cs["api_credits"]
	if grant.Amount != 499 {
		t.Fatalf("unexpected amount: %d", grant.Amount)
	}
	if grant.ExpiresDays == nil || *grant.ExpiresDays != 10 {
		t.Fatalf("unexpected expiry: %v", grant.ExpiresDays)
	}
	if grant.Cadence != CreditGrantCadencePerRenewal {
		t.Fatalf("unexpected cadence: %s", grant.Cadence)
	}
}

func TestPrice_GetCCBillFlexForm_LegacyPriceIDFallback(t *testing.T) {
	price := &Price{Processors: map[string]map[string]string{
		string(ProcessorCCBill): {
			ProcessorKeyCCBillFormName: "legacy-form",
			ProcessorKeyStripePriceID:  "legacy-flex-id",
		},
	}}

	formName, flexID, ok := price.GetCCBillFlexForm()
	if !ok {
		t.Fatal("expected legacy CCBill price_id config to be accepted")
	}
	if formName != "legacy-form" || flexID != "legacy-flex-id" {
		t.Fatalf("unexpected CCBill config: form=%q flex=%q", formName, flexID)
	}
}

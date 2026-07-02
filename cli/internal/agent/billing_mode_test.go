package agent

import "testing"

func TestBillingModeForClassifiesHarnessesAndEndpoints(t *testing.T) {
	cases := []struct {
		name    string
		harness string
		surface string
		baseURL string
		want    string
	}{
		{name: "claude code subscription", harness: "claude", want: BillingModeSubscription},
		{name: "codex subscription", harness: "codex", want: BillingModeSubscription},
		{name: "gemini cli subscription", harness: "gemini-cli", want: BillingModeSubscription},
		{name: "openrouter paid", harness: "openrouter", want: BillingModePaid},
		{name: "openai api paid", harness: "openai", want: BillingModePaid},
		{name: "anthropic api paid", harness: "anthropic", want: BillingModePaid},
		{name: "embedded agent local", harness: "agent", want: BillingModeLocal},
		{name: "openai compat localhost local", surface: "openai-compat", baseURL: "http://localhost:1234/v1", want: BillingModeLocal},
		{name: "openai compat 127 local", surface: "openai-compat", baseURL: "http://127.0.0.1:1234/v1", want: BillingModeLocal},
		{name: "openai compat 192 private local", surface: "openai-compat", baseURL: "http://192.168.1.10:1234/v1", want: BillingModeLocal},
		{name: "openai compat 10 private local", surface: "openai-compat", baseURL: "http://10.0.0.5:1234/v1", want: BillingModeLocal},
		{name: "openai compat public paid", surface: "openai-compat", baseURL: "https://api.example.com/v1", want: BillingModePaid},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := billingModeFor(tc.harness, tc.surface, tc.baseURL); got != tc.want {
				t.Fatalf("billingModeFor(%q, %q, %q) = %q, want %q", tc.harness, tc.surface, tc.baseURL, got, tc.want)
			}
		})
	}
}

func TestValidateBillingMode(t *testing.T) {
	for _, mode := range []string{BillingModePaid, BillingModeSubscription, BillingModeLocal} {
		if !ValidateBillingMode(mode) {
			t.Fatalf("ValidateBillingMode(%q) = false, want true", mode)
		}
	}
	if ValidateBillingMode("free") {
		t.Fatal("ValidateBillingMode(\"free\") = true, want false")
	}
}

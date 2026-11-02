package main

import (
	"context"
	"flag"
	"os"
	"testing"
	"time"

	"github.com/ringzerosystems/omnic/internal/repository/memory"
	"github.com/ringzerosystems/omnic/internal/service"
)

func TestParseAccountsAndNumbers(t *testing.T) {
	accounts := parseTwilioAccounts([]string{"AC123:token1", "AC456:token2"})
	if len(accounts) != 2 {
		t.Fatalf("expected 2 accounts, got %d", len(accounts))
	}
	if accounts[0].SID != "AC123" || accounts[0].Token != "token1" {
		t.Errorf("unexpected account[0]: %+v", accounts[0])
	}

	store := memory.NewStore()
	phoneSvc := service.NewPhoneNumberService(store)

	provisionConfig(context.Background(), phoneSvc, accounts, []string{"+14155550199", "+12125550100@AC456"}, provisionTarget{URL: "http://localhost:8000/webhook", Method: "POST"})

	pn1, err := store.FindPhoneNumberByNumber(context.Background(), "+14155550199")
	if err != nil {
		t.Fatalf("failed to find pn1: %v", err)
	}
	if pn1.AccountSID != "AC123" {
		t.Errorf("expected pn1 account AC123, got %s", pn1.AccountSID)
	}
	if pn1.SmsURL != "http://localhost:8000/webhook" {
		t.Errorf("expected SmsURL http://localhost:8000/webhook, got %s", pn1.SmsURL)
	}

	pn2, err := store.FindPhoneNumberByNumber(context.Background(), "+12125550100")
	if err != nil {
		t.Fatalf("failed to find pn2: %v", err)
	}
	if pn2.AccountSID != "AC456" {
		t.Errorf("expected pn2 account AC456, got %s", pn2.AccountSID)
	}
}

func TestDefaultConfig(t *testing.T) {
	accounts := parseTwilioAccounts(nil)
	if len(accounts) != 1 {
		t.Fatalf("expected default 1 account, got %d", len(accounts))
	}
	if accounts[0].SID != "AC00000000000000000000000000000001" {
		t.Errorf("expected default account SID, got %s", accounts[0].SID)
	}

	store := memory.NewStore()
	phoneSvc := service.NewPhoneNumberService(store)

	numbers := parseTwilioNumbers(nil)
	provisionConfig(context.Background(), phoneSvc, accounts, numbers, provisionTarget{URL: "", Method: "POST"})
	pn, err := store.FindPhoneNumberByNumber(context.Background(), "+14155550199")
	if err != nil {
		t.Fatalf("expected default number to be provisioned: %v", err)
	}
	if pn.AccountSID != "AC00000000000000000000000000000001" {
		t.Errorf("expected default account, got %s", pn.AccountSID)
	}
}

func TestParseFlagsWithEnvVars(t *testing.T) {
	t.Setenv("TWEMU_ADDR", ":9099")
	t.Setenv("TWEMU_PPROF", "localhost:6061")
	t.Setenv("TWEMU_DEV", "true")
	t.Setenv("TWEMU_DB", "/tmp/test.db")
	t.Setenv("TWEMU_TWILIO_AUTH_TOKEN", "custom-token")
	t.Setenv("TWEMU_SEED", "true")
	t.Setenv("TWEMU_TWILIO_UPSTREAM_URL", "http://agent.local/inbound")
	t.Setenv("TWEMU_TWILIO_UPSTREAM_METHOD", "GET")
	t.Setenv("TWEMU_DELIVERY_DELAY", "250ms")
	t.Setenv("TWEMU_TWILIO_ACCOUNTS", "AC100:tok100,AC200:tok200")
	t.Setenv("TWEMU_TWILIO_NUMBERS", "+15550001,+15550002")
	t.Setenv("TWEMU_FAIL_RULES", "send=429:20429@after 5;webhook=timeout")

	fs := flag.NewFlagSet("test-env", flag.ContinueOnError)
	cfg, showVersion, err := parseFlags(fs, nil)
	if err != nil {
		t.Fatalf("unexpected error parsing flags: %v", err)
	}
	if showVersion {
		t.Errorf("expected showVersion=false")
	}
	if cfg.addr != ":9099" || cfg.pprofAddr != "localhost:6061" || !cfg.dev || !cfg.seed {
		t.Errorf("mismatched basic config: %+v", cfg)
	}
	if cfg.dbPath != "/tmp/test.db" || cfg.twilioAuthToken != "custom-token" {
		t.Errorf("mismatched db/auth config: %+v", cfg)
	}
	if cfg.twilioUpstreamURL != "http://agent.local/inbound" || cfg.twilioUpstreamMethod != "GET" {
		t.Errorf("mismatched upstream config: %+v", cfg)
	}
	if cfg.deliveryDelay != 250*time.Millisecond {
		t.Errorf("expected delay 250ms, got %v", cfg.deliveryDelay)
	}
	if len(cfg.twilioAccounts) != 2 || cfg.twilioAccounts[0].SID != "AC100" {
		t.Errorf("mismatched accounts: %+v", cfg.twilioAccounts)
	}
	if len(cfg.twilioNumbers) != 2 || cfg.twilioNumbers[0] != "+15550001" {
		t.Errorf("mismatched numbers: %+v", cfg.twilioNumbers)
	}
	if len(cfg.failRules) != 2 || cfg.failRules[0] != "send=429:20429@after 5" {
		t.Errorf("mismatched failRules: %+v", cfg.failRules)
	}
}

func TestParseFlagsWithOmnicEnvVars(t *testing.T) {
	t.Setenv("TWEMU_ADDR", ":9099")
	t.Setenv("OMNIC_ADDR", ":7077")
	t.Setenv("OMNIC_DEV", "true")
	t.Setenv("OMNIC_TWILIO_AUTH_TOKEN", "omnic-token")

	fs := flag.NewFlagSet("test-omnic-env", flag.ContinueOnError)
	cfg, _, err := parseFlags(fs, nil)
	if err != nil {
		t.Fatalf("unexpected error parsing flags: %v", err)
	}
	if cfg.addr != ":7077" {
		t.Errorf("expected OMNIC_ADDR :7077 to take precedence over TWEMU_ADDR, got %s", cfg.addr)
	}
	if !cfg.dev {
		t.Errorf("expected OMNIC_DEV true")
	}
	if cfg.twilioAuthToken != "omnic-token" {
		t.Errorf("expected twilioAuthToken omnic-token, got %s", cfg.twilioAuthToken)
	}
}

func TestParseFlagsTwilioCLI(t *testing.T) {
	fs := flag.NewFlagSet("test-cli", flag.ContinueOnError)
	cfg, _, err := parseFlags(fs, []string{
		"-twilio-auth-token", "mytoken",
		"-twilio-upstream-url", "https://example.com/sms",
		"-twilio-upstream-method", "GET",
		"-twilio-account", "AC999:secret",
		"-twilio-number", "+15559990000@AC999",
	})
	if err != nil {
		t.Fatalf("unexpected error parsing flags: %v", err)
	}
	if cfg.twilioAuthToken != "mytoken" {
		t.Errorf("expected twilioAuthToken mytoken, got %s", cfg.twilioAuthToken)
	}
	if cfg.twilioUpstreamURL != "https://example.com/sms" {
		t.Errorf("expected twilioUpstreamURL https://example.com/sms, got %s", cfg.twilioUpstreamURL)
	}
	if cfg.twilioUpstreamMethod != "GET" {
		t.Errorf("expected twilioUpstreamMethod GET, got %s", cfg.twilioUpstreamMethod)
	}
	if len(cfg.twilioAccounts) != 1 || cfg.twilioAccounts[0].SID != "AC999" || cfg.twilioAccounts[0].Token != "secret" {
		t.Errorf("unexpected twilioAccounts: %+v", cfg.twilioAccounts)
	}
	if len(cfg.twilioNumbers) != 1 || cfg.twilioNumbers[0] != "+15559990000@AC999" {
		t.Errorf("unexpected twilioNumbers: %+v", cfg.twilioNumbers)
	}
}

func TestParseFlagsOverrides(t *testing.T) {
	t.Setenv("TWEMU_ADDR", ":9099")
	t.Setenv("TWEMU_DEV", "false")

	fs := flag.NewFlagSet("test-override", flag.ContinueOnError)
	cfg, _, err := parseFlags(fs, []string{"-addr", ":8888", "-dev"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.addr != ":8888" {
		t.Errorf("expected flag to override env var: got %s", cfg.addr)
	}
	if !cfg.dev {
		t.Errorf("expected flag -dev to override env var TWEMU_DEV=false")
	}

	t.Setenv("TWEMU_VERSION", "true")
	fsVer := flag.NewFlagSet("test-ver", flag.ContinueOnError)
	_, showVer, err := parseFlags(fsVer, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !showVer {
		t.Errorf("expected TWEMU_VERSION to set showVersion=true")
	}
}

func TestLoadEnvFile(t *testing.T) {
	tmpDir := t.TempDir()
	envPath := tmpDir + "/.env"
	content := "TWEMU_TEST_KEY=test_value\n# Comment\nINVALID_LINE\nTWEMU_TEST_QUOTED=\"quoted_val\"\n"
	if err := os.WriteFile(envPath, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write test env file: %v", err)
	}

	loadEnvFile(envPath)

	if val := os.Getenv("TWEMU_TEST_KEY"); val != "test_value" {
		t.Errorf("expected test_value, got %s", val)
	}
	if val := os.Getenv("TWEMU_TEST_QUOTED"); val != "quoted_val" {
		t.Errorf("expected quoted_val, got %s", val)
	}

	// Should not overwrite existing env var
	t.Setenv("TWEMU_EXISTING", "original")
	overridePath := tmpDir + "/.env.override"
	_ = os.WriteFile(overridePath, []byte("TWEMU_EXISTING=new_val"), 0o600)
	loadEnvFile(overridePath)
	if val := os.Getenv("TWEMU_EXISTING"); val != "original" {
		t.Errorf("expected original, got %s", val)
	}
}

func TestParseEngineFlags(t *testing.T) {
	t.Parallel()

	cases := []struct {
		raw      string
		twilio   bool
		whatsapp bool
		telegram bool
	}{
		{"", true, true, true},
		{"all", true, true, true},
		{"twilio", true, false, false},
		{"whatsapp", false, true, false},
		{"telegram", false, false, true},
		{"twilio,whatsapp", true, true, false},
		{"twilio,telegram", true, false, true},
		{"whatsapp,telegram", false, true, true},
		{"twilio,whatsapp,telegram", true, true, true},
	}

	for _, tc := range cases {
		ef := parseEngineFlags(tc.raw)
		if ef.enableTwilio != tc.twilio || ef.enableWhatsApp != tc.whatsapp || ef.enableTelegram != tc.telegram {
			t.Errorf("parseEngineFlags(%q) = %+v, want twilio=%v, wa=%v, tg=%v", tc.raw, ef, tc.twilio, tc.whatsapp, tc.telegram)
		}
	}
}

func TestParseTelegramFlags(t *testing.T) {
	t.Setenv("TWEMU_ENGINES", "telegram")
	t.Setenv("TWEMU_TELEGRAM_BOT_TOKEN", "my_bot_token")
	t.Setenv("TWEMU_TELEGRAM_BOT_USERNAME", "custom_bot")
	t.Setenv("TWEMU_TELEGRAM_SECRET_TOKEN", "my_secret")
	t.Setenv("TWEMU_TELEGRAM_DELIVERY_DELAY", "15ms")

	fs := flag.NewFlagSet("test-tg", flag.ContinueOnError)
	cfg, _, err := parseFlags(fs, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.engines != "telegram" {
		t.Errorf("expected engines=telegram, got %s", cfg.engines)
	}
	if cfg.telegramBotToken != "my_bot_token" || cfg.telegramBotUsername != "custom_bot" || cfg.telegramSecretToken != "my_secret" {
		t.Errorf("unexpected tg config: %+v", cfg)
	}
	if cfg.telegramDelay != 15*time.Millisecond {
		t.Errorf("expected delay 15ms, got %v", cfg.telegramDelay)
	}
}

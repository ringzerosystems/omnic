// Command omnic runs the HTTP API.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"

	"github.com/ringzerosystems/omnic/internal/handler"
	"github.com/ringzerosystems/omnic/internal/repository"
	"github.com/ringzerosystems/omnic/internal/repository/memory"
	"github.com/ringzerosystems/omnic/internal/repository/sqlite"
	"github.com/ringzerosystems/omnic/internal/service"
)

var version = "dev-dirty"

type stringSlice []string

func (s *stringSlice) String() string {
	return strings.Join(*s, ",")
}

func (s *stringSlice) Set(val string) error {
	*s = append(*s, val)
	return nil
}

type accountConfig struct {
	SID   string
	Token string
}

type engineFlags struct {
	enableTwilio   bool
	enableWhatsApp bool
	enableTelegram bool
}

func parseEngineFlags(raw string) engineFlags {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "" || raw == "all" {
		return engineFlags{enableTwilio: true, enableWhatsApp: true, enableTelegram: true}
	}
	flags := engineFlags{}
	for part := range strings.SplitSeq(raw, ",") {
		switch strings.TrimSpace(part) {
		case "twilio":
			flags.enableTwilio = true
		case "whatsapp":
			flags.enableWhatsApp = true
		case "telegram":
			flags.enableTelegram = true
		}
	}
	return flags
}

type serverConfig struct {
	addr                 string
	pprofAddr            string
	dbPath               string
	twilioAuthToken      string
	twilioUpstreamURL    string
	twilioUpstreamMethod string
	engines              string
	whatsappPhoneID      string
	whatsappWABAID       string
	whatsappToken        string
	whatsappVerifyToken  string
	whatsappSecret       string
	whatsappUpstreamURL  string
	telegramBotToken     string
	telegramBotUsername  string
	telegramBotName      string
	telegramUpstreamURL  string
	telegramSecretToken  string
	twilioAccounts       []accountConfig
	twilioNumbers        []string
	failRules            []string
	deliveryDelay        time.Duration
	telegramDelay        time.Duration
	dev                  bool
	seed                 bool
}

func envOrDefault(primary, fallbackEnv, defaultVal string) string {
	if val := os.Getenv(primary); val != "" {
		return val
	}
	if fallbackEnv != "" {
		if val := os.Getenv(fallbackEnv); val != "" {
			return val
		}
	}
	if after, ok := strings.CutPrefix(primary, "OMNIC_"); ok {
		legacy := "TWEMU_" + after
		if val := os.Getenv(legacy); val != "" {
			return val
		}
	}
	return defaultVal
}

func envBool(primary, fallbackEnv string, defaultVal bool) bool {
	val := os.Getenv(primary)
	if val == "" && fallbackEnv != "" {
		val = os.Getenv(fallbackEnv)
	}
	if val == "" && strings.HasPrefix(primary, "OMNIC_") {
		val = os.Getenv("TWEMU_" + strings.TrimPrefix(primary, "OMNIC_"))
	}
	if val == "" {
		return defaultVal
	}
	b, err := strconv.ParseBool(val)
	if err != nil {
		return defaultVal
	}
	return b
}

func parseTwilioAccounts(raw []string) []accountConfig {
	if len(raw) == 0 {
		env := envOrDefault("OMNIC_TWILIO_ACCOUNTS", "TWILIO_ACCOUNTS", "")
		if env == "" {
			env = envOrDefault("OMNIC_TWILIO_ACCOUNT", "TWILIO_ACCOUNT", "")
		}
		if env != "" {
			for part := range strings.SplitSeq(env, ",") {
				if trimmed := strings.TrimSpace(part); trimmed != "" {
					raw = append(raw, trimmed)
				}
			}
		}
	}
	if len(raw) == 0 {
		return []accountConfig{{SID: "AC00000000000000000000000000000001", Token: "authtoken"}}
	}
	res := make([]accountConfig, 0, len(raw))
	for _, entry := range raw {
		sid, token, found := strings.Cut(entry, ":")
		if !found {
			token = "authtoken"
		}
		res = append(res, accountConfig{SID: strings.TrimSpace(sid), Token: strings.TrimSpace(token)})
	}
	return res
}

func parseTwilioNumbers(raw []string) []string {
	if len(raw) == 0 {
		env := envOrDefault("OMNIC_TWILIO_NUMBERS", "TWILIO_NUMBERS", "")
		if env == "" {
			env = envOrDefault("OMNIC_TWILIO_NUMBER", "TWILIO_NUMBER", "")
		}
		if env != "" {
			for part := range strings.SplitSeq(env, ",") {
				if trimmed := strings.TrimSpace(part); trimmed != "" {
					raw = append(raw, trimmed)
				}
			}
		}
	}
	if len(raw) == 0 {
		return []string{"+14155550199"}
	}
	return raw
}

func parseFailRules(raw []string) []string {
	if len(raw) == 0 {
		env := envOrDefault("OMNIC_FAIL_RULES", "TWILIO_EMU_FAIL_RULES", "")
		if env == "" {
			env = envOrDefault("OMNIC_FAIL_RULE", "TWEMU_FAIL_RULE", "")
		}
		if env != "" {
			sep := ","
			if strings.Contains(env, ";") {
				sep = ";"
			}
			for part := range strings.SplitSeq(env, sep) {
				if trimmed := strings.TrimSpace(part); trimmed != "" {
					raw = append(raw, trimmed)
				}
			}
		}
	}
	return raw
}

type provisionTarget struct {
	URL    string
	Method string
}

func provisionConfig(ctx context.Context, phoneSvc *service.PhoneNumberService, accounts []accountConfig, numbers []string, target provisionTarget) {
	if len(accounts) == 0 {
		return
	}
	defaultAccount := accounts[0].SID
	for _, numEntry := range numbers {
		num := numEntry
		acct := defaultAccount
		if n, a, found := strings.Cut(numEntry, "@"); found {
			num = strings.TrimSpace(n)
			acct = strings.TrimSpace(a)
		}
		_, _ = phoneSvc.Provision(ctx, acct, service.ProvisionParams{
			PhoneNumber:  num,
			FriendlyName: "Auto-provisioned " + num,
			SmsURL:       target.URL,
			SmsMethod:    target.Method,
		})
	}
}

func main() {
	os.Exit(run())
}

type providerFlagHolders struct {
	twilioAuthToken      *string
	twilioUpstreamURL    *string
	twilioUpstreamMethod *string
	waPhoneID            *string
	waWABAID             *string
	waToken              *string
	waVerifyToken        *string
	waSecret             *string
	waUpstreamURL        *string
	tgToken              *string
	tgUsername           *string
	tgName               *string
	tgUpstreamURL        *string
	tgSecretToken        *string
	tgDelayStr           *string
}

func registerProviderFlags(fs *flag.FlagSet) providerFlagHolders {
	return providerFlagHolders{
		twilioAuthToken:      fs.String("twilio-auth-token", envOrDefault("OMNIC_TWILIO_AUTH_TOKEN", "TWILIO_AUTH_TOKEN", "authtoken"), "secret token for Twilio HMAC-SHA1 signature calculation"),
		twilioUpstreamURL:    fs.String("twilio-upstream-url", envOrDefault("OMNIC_TWILIO_UPSTREAM_URL", "TWILIO_UPSTREAM_URL", ""), "default webhook URL where inbound Twilio SMS messages are forwarded"),
		twilioUpstreamMethod: fs.String("twilio-upstream-method", envOrDefault("OMNIC_TWILIO_UPSTREAM_METHOD", "TWILIO_UPSTREAM_METHOD", "POST"), "HTTP method for Twilio upstream forwarding (POST or GET)"),
		waPhoneID:            fs.String("whatsapp-phone-id", envOrDefault("OMNIC_WHATSAPP_PHONE_ID", "WHATSAPP_PHONE_ID", "100000000000001"), "WhatsApp Phone Number ID"),
		waWABAID:             fs.String("whatsapp-waba-id", envOrDefault("OMNIC_WHATSAPP_WABA_ID", "WHATSAPP_WABA_ID", "200000000000001"), "WhatsApp Business Account ID"),
		waToken:              fs.String("whatsapp-token", envOrDefault("OMNIC_WHATSAPP_TOKEN", "WHATSAPP_TOKEN", "wa_token_dev"), "WhatsApp Bearer access token"),
		waVerifyToken:        fs.String("whatsapp-verify-token", envOrDefault("OMNIC_WHATSAPP_VERIFY_TOKEN", "WHATSAPP_VERIFY_TOKEN", "wa_verify_token"), "WhatsApp Webhook verification token"),
		waSecret:             fs.String("whatsapp-secret", envOrDefault("OMNIC_WHATSAPP_SECRET", "WHATSAPP_SECRET", "wa_secret_dev"), "WhatsApp app secret for HMAC"),
		waUpstreamURL:        fs.String("whatsapp-upstream-url", envOrDefault("OMNIC_WHATSAPP_UPSTREAM_URL", "WHATSAPP_UPSTREAM_URL", ""), "WhatsApp dedicated upstream webhook URL"),
		tgToken:              fs.String("telegram-bot-token", envOrDefault("OMNIC_TELEGRAM_BOT_TOKEN", "TELEGRAM_BOT_TOKEN", "123456789:mock_token_dev"), "Mock Telegram Bot token"),
		tgUsername:           fs.String("telegram-bot-username", envOrDefault("OMNIC_TELEGRAM_BOT_USERNAME", "TELEGRAM_BOT_USERNAME", "omnic_bot"), "Bot username"),
		tgName:               fs.String("telegram-bot-name", envOrDefault("OMNIC_TELEGRAM_BOT_NAME", "TELEGRAM_BOT_NAME", "OmnicBot"), "Bot display name"),
		tgUpstreamURL:        fs.String("telegram-upstream-url", envOrDefault("OMNIC_TELEGRAM_UPSTREAM_URL", "TELEGRAM_UPSTREAM_URL", ""), "Telegram dedicated upstream webhook URL"),
		tgSecretToken:        fs.String("telegram-secret-token", envOrDefault("OMNIC_TELEGRAM_SECRET_TOKEN", "TELEGRAM_SECRET_TOKEN", ""), "Secret token for X-Telegram-Bot-Api-Secret-Token"),
		tgDelayStr:           fs.String("telegram-delivery-delay", envOrDefault("OMNIC_TELEGRAM_DELIVERY_DELAY", "TELEGRAM_DELIVERY_DELAY", "20ms"), "Simulated Telegram delivery latency"),
	}
}

func parseFlags(fs *flag.FlagSet, args []string) (serverConfig, bool, error) {
	addr := fs.String("addr", envOrDefault("OMNIC_ADDR", "TWILIO_EMU_ADDR", ":8080"), "listen address")
	pprofAddr := fs.String("pprof", envOrDefault("OMNIC_PPROF", "TWILIO_EMU_PPROF", "localhost:6060"), "internal listen address for pprof and /metrics (empty disables)")
	dev := fs.Bool("dev", envBool("OMNIC_DEV", "TWILIO_EMU_DEV", false), "human-readable development logging")
	versionFlag := fs.Bool("version", envBool("OMNIC_VERSION", "TWILIO_EMU_VERSION", false), "print version and exit")
	dbPath := fs.String("db", envOrDefault("OMNIC_DB", "TWILIO_EMU_DB", ""), "path to SQLite database file (empty uses in-memory)")
	seed := fs.Bool("seed", envBool("OMNIC_SEED", "TWILIO_EMU_SEED", false), "seed default phone numbers on startup")
	deliveryDelayStr := fs.String("delivery-delay", envOrDefault("OMNIC_DELIVERY_DELAY", "TWILIO_EMU_DELIVERY_DELAY", "50ms"), "simulated carrier transit time")
	engines := fs.String("engines", envOrDefault("OMNIC_ENGINES", "TWILIO_EMU_ENGINES", "twilio"), "enabled engines: twilio, whatsapp, telegram, or all/comma-separated")

	pf := registerProviderFlags(fs)

	var twilioAccountFlags, twilioNumberFlags, failFlags stringSlice
	fs.Var(&twilioAccountFlags, "twilio-account", "Twilio account credentials AC<hex>:TOKEN (repeatable)")
	fs.Var(&twilioNumberFlags, "twilio-number", "pre-provisioned Twilio phone number +E164[@AC...] (repeatable)")
	fs.Var(&failFlags, "fail-rule", "failure injection rules (repeatable)")

	if err := fs.Parse(args); err != nil {
		return serverConfig{}, false, err
	}
	if *versionFlag {
		return serverConfig{}, true, nil
	}

	delay, _ := time.ParseDuration(*deliveryDelayStr)
	if delay <= 0 {
		delay = 50 * time.Millisecond
	}

	tgDelay, _ := time.ParseDuration(*pf.tgDelayStr)
	if tgDelay <= 0 {
		tgDelay = 20 * time.Millisecond
	}

	waUpstream := *pf.waUpstreamURL
	if waUpstream == "" {
		waUpstream = *pf.twilioUpstreamURL
	}

	tgUpstream := *pf.tgUpstreamURL
	if tgUpstream == "" {
		tgUpstream = *pf.twilioUpstreamURL
	}

	cfg := serverConfig{
		addr:                 *addr,
		pprofAddr:            *pprofAddr,
		dbPath:               *dbPath,
		twilioAuthToken:      *pf.twilioAuthToken,
		twilioUpstreamURL:    *pf.twilioUpstreamURL,
		twilioUpstreamMethod: *pf.twilioUpstreamMethod,
		engines:              *engines,
		whatsappPhoneID:      *pf.waPhoneID,
		whatsappWABAID:       *pf.waWABAID,
		whatsappToken:        *pf.waToken,
		whatsappVerifyToken:  *pf.waVerifyToken,
		whatsappSecret:       *pf.waSecret,
		whatsappUpstreamURL:  waUpstream,
		telegramBotToken:     *pf.tgToken,
		telegramBotUsername:  *pf.tgUsername,
		telegramBotName:      *pf.tgName,
		telegramUpstreamURL:  tgUpstream,
		telegramSecretToken:  *pf.tgSecretToken,
		deliveryDelay:        delay,
		telegramDelay:        tgDelay,
		twilioAccounts:       parseTwilioAccounts(twilioAccountFlags),
		twilioNumbers:        parseTwilioNumbers(twilioNumberFlags),
		failRules:            parseFailRules(failFlags),
		dev:                  *dev,
		seed:                 *seed,
	}
	return cfg, false, nil
}

func loadEnvFile(path string) {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		val = strings.Trim(val, `"'`)
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, val)
		}
	}
}

// run holds all the defers so they execute before main's os.Exit.
func run() int {
	loadEnvFile(".env")
	cfg, showVersion, err := parseFlags(flag.CommandLine, os.Args[1:])
	if err != nil {
		return 1
	}
	if showVersion {
		fmt.Println(version)
		return 0
	}
	return startServer(cfg)
}

func startServer(cfg serverConfig) int {
	logger := newLogger(cfg.dev)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	e := echo.New()
	e.HideBanner = true
	e.HidePort = true
	reg := prometheus.NewRegistry()
	reg.MustRegister(
		collectors.NewGoCollector(collectors.WithGoCollectorRuntimeMetrics(collectors.MetricsAll)),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
	e.Use(middleware.Recover(), requestLogger(logger), handler.Metrics(reg))
	e.Server.ReadHeaderTimeout = 5 * time.Second
	e.Server.ReadTimeout = 10 * time.Second
	e.Server.WriteTimeout = 10 * time.Second
	e.Server.IdleTimeout = 60 * time.Second

	handler.RegisterHealthRoutes(e)

	h, closer := initHandler(cfg, logger)
	if closer != nil {
		defer func() { _ = closer.Close() }()
	}

	ef := parseEngineFlags(cfg.engines)
	if ef.enableTwilio {
		h.RegisterTwilioRoutes(e)
	}
	if ef.enableWhatsApp {
		h.RegisterWhatsAppRoutes(e)
	}
	if ef.enableTelegram {
		h.RegisterTelegramRoutes(e)
	}
	h.RegisterEmuRoutes(e)

	go startHTTP(e, cfg.addr, logger, stop)
	pprofSrv := startPprof(cfg.pprofAddr, reg, logger, stop)

	<-ctx.Done()
	logger.Info("shutting down")

	return performShutdown(e, pprofSrv, logger)
}

func startHTTP(e *echo.Echo, addr string, logger *zap.Logger, stop context.CancelFunc) {
	logger.Info("http listening", zap.String("addr", addr))
	if err := e.Start(addr); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("server stopped", zap.Error(err))
		stop()
	}
}

func startPprof(pprofAddr string, reg *prometheus.Registry, logger *zap.Logger, stop context.CancelFunc) *http.Server {
	if pprofAddr == "" {
		return nil
	}
	srv := &http.Server{Addr: pprofAddr, Handler: internalMux(reg), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		logger.Info("internal listening", zap.String("addr", pprofAddr))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("internal stopped", zap.Error(err))
			stop()
		}
	}()
	return srv
}

func performShutdown(e *echo.Echo, pprofSrv *http.Server, logger *zap.Logger) int {
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	exit := 0
	if err := e.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown", zap.Error(err))
		exit = 1
	}
	if pprofSrv != nil {
		if err := pprofSrv.Shutdown(shutdownCtx); err != nil {
			logger.Error("pprof shutdown", zap.Error(err))
			exit = 1
		}
	}
	_ = logger.Sync()
	return exit
}

func initHandler(cfg serverConfig, logger *zap.Logger) (*handler.Handler, io.Closer) {
	var (
		store  repository.Store
		closer io.Closer
	)
	if cfg.dbPath != "" {
		db, err := sqlite.Open(cfg.dbPath)
		if err != nil {
			logger.Fatal("sqlite init failed", zap.Error(err))
		}
		store = db
		closer = db
	} else {
		store = memory.NewStore()
	}

	phoneSvc := service.NewPhoneNumberService(store)
	msgSvc := service.NewMessageService(store)
	svcSvc := service.NewMessagingServiceService(store)
	dispatcher := service.NewWebhookDispatcher(store, cfg.twilioAuthToken)
	dispatcher.SetDefaultUpstream(cfg.twilioUpstreamURL, cfg.twilioUpstreamMethod)
	dispatcher.SetDeliveryDelay(cfg.deliveryDelay)

	broker := service.NewMemoryEventBroker()
	msgSvc.SetBroker(broker)
	dispatcher.SetBroker(broker)

	sim := service.NewSimulationService()
	for _, rawRule := range cfg.failRules {
		if rule, err := sim.ParseRuleString(rawRule); err == nil {
			sim.AddRule(rule)
		} else {
			logger.Warn("invalid fail rule", zap.String("rule", rawRule), zap.Error(err))
		}
	}
	msgSvc.SetSimulation(sim)
	dispatcher.SetSimulation(sim)

	if cfg.seed {
		seedNumbers(phoneSvc)
	}
	provisionConfig(context.Background(), phoneSvc, cfg.twilioAccounts, cfg.twilioNumbers, provisionTarget{URL: cfg.twilioUpstreamURL, Method: cfg.twilioUpstreamMethod})

	h := handler.NewHandler(store, phoneSvc, msgSvc, svcSvc, dispatcher)
	h.SetBroker(broker)
	h.SetSimulation(sim)

	ef := parseEngineFlags(cfg.engines)
	if ef.enableWhatsApp {
		waCfg := service.WhatsAppConfig{
			PhoneNumberID: cfg.whatsappPhoneID,
			WABAID:        cfg.whatsappWABAID,
			VerifyToken:   cfg.whatsappVerifyToken,
			AppSecret:     cfg.whatsappSecret,
			BearerToken:   cfg.whatsappToken,
			UpstreamURL:   cfg.whatsappUpstreamURL,
			DeliveryDelay: cfg.deliveryDelay,
		}
		waSvc := service.NewWhatsAppService(store, broker, waCfg)
		h.SetWhatsApp(waSvc)
	}
	if ef.enableTelegram {
		tgCfg := service.TelegramConfig{
			BotToken:      cfg.telegramBotToken,
			BotUsername:   cfg.telegramBotUsername,
			BotName:       cfg.telegramBotName,
			UpstreamURL:   cfg.telegramUpstreamURL,
			SecretToken:   cfg.telegramSecretToken,
			DeliveryDelay: cfg.telegramDelay,
		}
		tgSvc := service.NewTelegramService(store, broker, tgCfg)
		h.SetTelegram(tgSvc)
	}

	return h, closer
}

func seedNumbers(phoneSvc *service.PhoneNumberService) {
	ctx := context.Background()
	_, _ = phoneSvc.Provision(ctx, "AC001", service.ProvisionParams{
		PhoneNumber:  "+14155550199",
		FriendlyName: "Demo Support",
		SmsURL:       "http://localhost:3000/sms",
		SmsMethod:    "POST",
	})
}

// internalMux serves pprof and Prometheus metrics for the internal listener.
func internalMux(reg *prometheus.Registry) *http.ServeMux {
	// ponytail: sample every block/contention event; lower if profiling overhead shows up.
	runtime.SetBlockProfileRate(1)
	runtime.SetMutexProfileFraction(1)
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{Registry: reg}))
	return mux
}

func newLogger(dev bool) *zap.Logger {
	var logger *zap.Logger
	var err error
	if dev {
		logger, err = zap.NewDevelopment()
	} else {
		logger, err = zap.NewProduction()
	}
	if err != nil {
		panic(err) // cannot happen for the built-in configs; fail loud before serving
	}
	return logger
}

// requestLogger logs one line per request through zap (allocation-free typed fields).
func requestLogger(logger *zap.Logger) echo.MiddlewareFunc {
	return middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogMethod: true, LogURI: true, LogStatus: true, LogLatency: true, LogRemoteIP: true, LogError: true,
		LogValuesFunc: func(_ echo.Context, v middleware.RequestLoggerValues) error {
			logger.Info("request",
				zap.String("method", v.Method),
				zap.String("uri", v.URI),
				zap.Int("status", v.Status),
				zap.Duration("latency", v.Latency),
				zap.String("remote_ip", v.RemoteIP),
				zap.Error(v.Error),
			)
			return nil
		},
	})
}

package main

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gopanel/internal/config"
	"gopanel/internal/modules/customers"
	customershandler "gopanel/internal/modules/customers/handler"
	"gopanel/internal/modules/dashboard"
	dashboardhandler "gopanel/internal/modules/dashboard/handler"
	"gopanel/internal/modules/members"
	membershandler "gopanel/internal/modules/members/handler"
	"gopanel/internal/modules/products"
	productshandler "gopanel/internal/modules/products/handler"
	"gopanel/internal/modules/tenants"
	tenantshandler "gopanel/internal/modules/tenants/handler"
	"gopanel/internal/platform/auth"
	authhandler "gopanel/internal/platform/auth/handler"
	"gopanel/internal/platform/database"
	"gopanel/internal/platform/events"
	eventshandler "gopanel/internal/platform/events/handler"
	httpserver "gopanel/internal/platform/http"
	"gopanel/internal/platform/observability"
	metricshandler "gopanel/internal/platform/observability/handler"
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == "healthcheck" {
		os.Exit(probe())
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)
	if code := run(logger); code != 0 {
		os.Exit(code)
	}
}

func run(logger *slog.Logger) int {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		logger.Error("invalid_configuration", "error", err)
		return 1
	}
	db, err := database.Open(cfg)
	if err != nil {
		logger.Error("database_configuration_failed")
		return 1
	}
	defer db.Close()
	listener, err := net.Listen("tcp", cfg.HTTPAddr)
	if err != nil {
		logger.Error("listen_failed")
		return 1
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	service := auth.New(db, cfg, nil)
	recorder := observability.New()
	router := httpserver.NewRouter(db, logger, cfg.HealthTimeout, recorder)
	authHTTP := authhandler.New(service, cfg.Env == "production", logger)
	authHTTP.Register(router)
	broker := events.New()
	defer broker.Close()
	metricshandler.New(db, recorder, broker, authHTTP).Register(router)
	dashboardhandler.New(dashboard.New(db, nil), authHTTP).Register(router)
	eventshandler.New(broker, authHTTP, ctx).Register(router)
	customershandler.New(customers.New(db, broker), authHTTP).Register(router)
	productshandler.New(products.New(db, broker), authHTTP).Register(router)
	membershandler.New(members.New(db), authHTTP).Register(router)
	tenantshandler.New(tenants.New(db), authHTTP).Register(router)
	maintenanceDone := make(chan struct{})
	go func() { defer close(maintenanceDone); service.Maintain(ctx, logger) }()
	defer func() { stop(); <-maintenanceDone }()
	server := httpserver.NewServer(cfg.HTTPAddr, router)
	logger.Info("server_started", "addr", listener.Addr().String(), "environment", cfg.Env)
	if err := httpserver.Serve(ctx, server, listener, cfg.ShutdownTimeout, logger); err != nil {
		logger.Error("server_failed", "error", err)
		return 1
	}
	return 0
}

func probe() int {
	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return 1
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://" + net.JoinHostPort(host, port) + "/health/ready")
	if err != nil {
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}

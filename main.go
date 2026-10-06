package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"meth-enginev2/config"
	"meth-enginev2/controller"
	"meth-enginev2/db"
	"meth-enginev2/handler"
	"meth-enginev2/helper/applog"
	"meth-enginev2/helper/wizard"
	"meth-enginev2/model"
	"meth-enginev2/routes"
	"meth-enginev2/view"
)

func main() {
	configPath := flag.String("config", "", "path to config.yaml (default ./config.yaml or $METH_CONFIG)")
	check := flag.Bool("check", false, "validate the configuration and exit")
	health := flag.Bool("healthcheck", false, "ask the running engine whether it is healthy and exit 0 or 1")
	flag.Parse()
	if *health {
		if err := handler.Healthcheck(config.ListenAddr()); err != nil {
			fmt.Fprintln(os.Stderr, "unhealthy:", err)
			os.Exit(1)
		}
		fmt.Println("healthy")
		return
	}

	logOpts, err := config.LogOptions()
	if err != nil {
		fatal("config", err)
	}
	applog.Setup(logOpts, os.Stderr)
	cfg, err := config.Load(*configPath)
	if err != nil {
		fatal("config", err)
	}
	if *check {
		cfg.Report(os.Stdout)
		return
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dbCtx := context.Background()
	database, err := db.Open(dbCtx, cfg.DatabaseURL, applog.Tracer{Slow: cfg.Log.SlowQuery})
	if err != nil {
		fatal("startup", err)
	}
	go applog.Stats(ctx, cfg.Log.StatsInterval, database.Pool)
	if cfg.PprofAddr != "" {
		go handler.ServePprof(cfg.PprofAddr)
	}
	if err := database.Migrate(dbCtx); err != nil {
		fatal("startup", err)
	}

	store := model.New(database)
	if err := wizard.Seed(dbCtx, cfg, store); err != nil {
		fatal("startup", err)
	}
	app := controller.New(cfg, store, database.Probe)
	slog.Info("meth engine listening", "addr", cfg.Addr, "env", cfg.Env, "db_max_conns", database.Stat().MaxConns(),
		"protection", cfg.Protection(), "log_level", cfg.Log.Level.String())

	if err := handler.Serve(ctx, routes.Build(app, view.Static), cfg.Addr, handler.ShutdownGrace); err != nil {
		fatal("server stopped", err)
	}
	slog.Info("stop signal received; in-flight requests finished")
	database.Close()
	slog.Info("meth engine stopped")
}

func fatal(msg string, err error) {
	slog.Error(msg, "err", err)
	os.Exit(1)
}

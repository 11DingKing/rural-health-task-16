package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"ruralhealth/internal/accesspolicy"
	"ruralhealth/internal/audit"
	"ruralhealth/internal/callback"
	"ruralhealth/internal/clock"
	"ruralhealth/internal/config"
	"ruralhealth/internal/consistency"
	"ruralhealth/internal/dispatch"
	"ruralhealth/internal/gateway"
	"ruralhealth/internal/httpapi"
	"ruralhealth/internal/logging"
	"ruralhealth/internal/persistence"
	"ruralhealth/internal/quota"
	"ruralhealth/internal/repository"
	"ruralhealth/internal/rulemgmt"
	"ruralhealth/internal/scheduler"
	"ruralhealth/internal/sla"
	"ruralhealth/internal/submission"
)

func main() {
	configPath := flag.String("config", "configs/config.yaml", "path to config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}

	logger := logging.New(cfg.Logging, os.Stderr)
	clk := clock.Real()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// --- Persistence layer ---
	kvStore := persistence.NewKVStore(cfg.DataDir)
	if err := kvStore.Open(ctx); err != nil {
		logger.Error(ctx, "failed to open kv store", "error", err)
		os.Exit(1)
	}
	defer kvStore.Close()
	logger.Info(ctx, "kv store opened", "data_dir", cfg.DataDir, "version", kvStore.DataVersion())

	// --- Audit store (SQLite) ---
	auditStore, err := audit.NewStore(filepath.Join(cfg.DataDir, "audit.db"))
	if err != nil {
		logger.Error(ctx, "failed to open audit store", "error", err)
		os.Exit(1)
	}
	defer auditStore.Close()

	// --- Repositories ---
	store := repository.NewStore(kvStore)
	subRepo := repository.NewSubmissionRepo(store)
	ruleRepo := repository.NewRuleRepo(store)
	agencyRepo := repository.NewAgencyRepo(store)
	dispatchRepo := repository.NewDispatchRepo(store)
	resultRepo := repository.NewResultRepo(store)
	summaryRepo := repository.NewSummaryRepo(store)
	callbackRepo := repository.NewCallbackRepo(store)
	idemRepo := repository.NewIdempotencyRepo(store)

	// --- Services ---
	submissionSvc := submission.NewService(subRepo, ruleRepo, idemRepo, logger, clk)
	ruleSvc := rulemgmt.NewService(ruleRepo, logger, clk)
	gwManager := gateway.NewManager(cfg.Gateway, cfg.Upstreams, clk.Now)
	dispatchSvc := dispatch.NewService(dispatchRepo, subRepo, agencyRepo, resultRepo, idemRepo, gwManager, logger, clk)
	consistencySvc := consistency.NewService(summaryRepo, resultRepo, dispatchRepo, subRepo, logger, clk)
	callbackSvc := callback.NewService(callbackRepo, subRepo, callback.NewSigner(cfg.SigningKey), logger, clk, cfg.Callback)

	// --- Gateway manager (created above) ---

	// --- Audit service ---
	auditSvc := audit.NewService(auditStore, logger, clk)

	// --- Cross-cutting policy, quota, and SLA services ---
	accessEngine := accesspolicy.NewEngine(subRepo, dispatchRepo, clk)
	quotaSvc := quota.NewService(store, subRepo, auditSvc, logger, clk, quota.DefaultPolicy())
	slaSvc := sla.NewService(store, dispatchRepo, auditSvc, logger, clk, sla.DefaultPolicy())

	// Startup self-heal: reconcile persisted quota reservations and SLA
	// records against actual submission/dispatch state recovered from disk.
	if n, err := quotaSvc.Recompute(ctx); err != nil {
		logger.Error(ctx, "quota recompute on startup failed", "error", err)
	} else if n > 0 {
		logger.Info(ctx, "quota recompute reconciled reservations", "count", n)
	}
	if n, err := slaSvc.Recompute(ctx); err != nil {
		logger.Error(ctx, "sla recompute on startup failed", "error", err)
	} else if n > 0 {
		logger.Info(ctx, "sla recompute backfilled records", "count", n)
	}

	// --- Scheduler ---
	sched := scheduler.New(logger, clk)
	registerSchedulerTasks(sched, dispatchSvc, callbackSvc, consistencySvc, quotaSvc, slaSvc, cfg.Scheduler, logger)

	// --- Readiness deps ---
	readyDeps := &httpapi.ReadyDeps{
		CheckStore: func() bool {
			return kvStore.Ping(context.Background()) == nil
		},
		CheckAudit: func() bool {
			return auditSvc.Ping(context.Background()) == nil
		},
		CheckScheduler: func() bool {
			return sched.IsRunning()
		},
		CheckMigrations: func() bool {
			return kvStore.DataVersion() >= persistence.DataVersion
		},
	}

	// --- HTTP handlers ---
	handlers := &httpapi.Handlers{
		Submission:  submissionSvc,
		Dispatch:    dispatchSvc,
		Consistency: consistencySvc,
		Callback:    callbackSvc,
		RuleMgmt:    ruleSvc,
		Audit:       auditSvc,
		Gateway:     gwManager,
		Scheduler:   sched,
		Access:      accessEngine,
		Quota:       quotaSvc,
		SLA:         slaSvc,
		ReadyDeps:   readyDeps,
		Logger:      logger,
	}

	router := httpapi.NewRouter(handlers)
	server := httpapi.NewServer(cfg.Server, router, logger)

	// Start scheduler
	sched.Start(ctx)
	logger.Info(ctx, "scheduler started")

	// Handle signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		logger.Info(ctx, "shutdown signal received")
		cancel()
	}()

	if err := server.Start(ctx); err != nil {
		logger.Error(ctx, "server error", "error", err)
	}
	sched.Stop(cfg.Server.ShutdownTimeout)
	_ = server.Shutdown(context.Background(), cfg.Server.ShutdownTimeout)
	logger.Info(ctx, "server stopped")
}

func registerSchedulerTasks(sched *scheduler.Scheduler, dispatchSvc *dispatch.Service, callbackSvc *callback.Service, consistencySvc *consistency.Service, quotaSvc *quota.Service, slaSvc *sla.Service, cfg config.SchedulerConfig, logger logging.Logger) {
	sched.Register(&dispatchRetryTask{dispatchSvc: dispatchSvc, interval: cfg.DispatchInterval, logger: logger})
	sched.Register(&callbackRetryTask{callbackSvc: callbackSvc, interval: cfg.CallbackInterval, logger: logger})
	sched.Register(&consistencyCheckTask{consistencySvc: consistencySvc, interval: cfg.ConsistencyCheck, logger: logger})
	sched.Register(&quotaSweepTask{quotaSvc: quotaSvc, interval: cfg.ConsistencyCheck, logger: logger})
	sched.Register(&slaScanTask{slaSvc: slaSvc, interval: cfg.DispatchInterval, logger: logger})
}

type quotaSweepTask struct {
	quotaSvc *quota.Service
	interval time.Duration
	logger   logging.Logger
}

func (t *quotaSweepTask) Name() string            { return "quota_sweep" }
func (t *quotaSweepTask) Interval() time.Duration { return t.interval }
func (t *quotaSweepTask) Run(ctx context.Context) error {
	n, err := t.quotaSvc.SweepExpired(ctx)
	if err != nil {
		return err
	}
	if n > 0 {
		t.logger.Info(ctx, "quota sweep deleted stale reservations", "count", n)
	}
	return nil
}

type slaScanTask struct {
	slaSvc   *sla.Service
	interval time.Duration
	logger   logging.Logger
}

func (t *slaScanTask) Name() string            { return "sla_scan" }
func (t *slaScanTask) Interval() time.Duration { return t.interval }
func (t *slaScanTask) Run(ctx context.Context) error {
	n, err := t.slaSvc.ScanBreaches(ctx)
	if err != nil {
		return err
	}
	if n > 0 {
		t.logger.Info(ctx, "sla scan escalated breaches", "count", n)
	}
	return nil
}

type dispatchRetryTask struct {
	dispatchSvc *dispatch.Service
	interval    time.Duration
	logger      logging.Logger
}

func (t *dispatchRetryTask) Name() string            { return "dispatch_retry" }
func (t *dispatchRetryTask) Interval() time.Duration { return t.interval }
func (t *dispatchRetryTask) Run(ctx context.Context) error {
	count, err := t.dispatchSvc.RetryFailed(ctx)
	if err != nil {
		return err
	}
	if count > 0 {
		t.logger.Info(ctx, "dispatch retry task retried tasks", "count", count)
	}
	return nil
}

type callbackRetryTask struct {
	callbackSvc *callback.Service
	interval    time.Duration
	logger      logging.Logger
}

func (t *callbackRetryTask) Name() string            { return "callback_retry" }
func (t *callbackRetryTask) Interval() time.Duration { return t.interval }
func (t *callbackRetryTask) Run(ctx context.Context) error {
	count, err := t.callbackSvc.RetryPending(ctx)
	if err != nil {
		return err
	}
	if count > 0 {
		t.logger.Info(ctx, "callback retry task delivered callbacks", "count", count)
	}
	return nil
}

type consistencyCheckTask struct {
	consistencySvc *consistency.Service
	interval       time.Duration
	logger         logging.Logger
}

func (t *consistencyCheckTask) Name() string            { return "consistency_check" }
func (t *consistencyCheckTask) Interval() time.Duration { return t.interval }
func (t *consistencyCheckTask) Run(ctx context.Context) error {
	count, err := t.consistencySvc.RepairStale(ctx)
	if err != nil {
		return err
	}
	if count > 0 {
		t.logger.Info(ctx, "consistency check repaired summaries", "count", count)
	}
	return nil
}

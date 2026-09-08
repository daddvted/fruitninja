package main

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/caarlos0/env/v9"
	"github.com/daddvted/fruitninja/data"
	"github.com/daddvted/fruitninja/fruitninja"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var settings fruitninja.FruitNinjaSettings

//go:embed static/*
var staticFiles embed.FS

func createLogger(dev bool, level string) *zap.Logger {
	encoding := "json"
	callerDisabled := true
	stacktraceDisabled := true
	if dev {
		encoding = "console"
		callerDisabled = false
	}
	// Set log level to INFO by default
	lvl := zap.NewAtomicLevelAt(zap.InfoLevel)
	switch level {
	case "debug":
		lvl = zap.NewAtomicLevelAt(zap.DebugLevel)
	case "error":
		lvl = zap.NewAtomicLevelAt(zap.ErrorLevel)
	case "warn":
		lvl = zap.NewAtomicLevelAt(zap.WarnLevel)
	}
	encoderCfg := zap.NewProductionEncoderConfig()
	encoderCfg.TimeKey = "timestamp"
	encoderCfg.EncodeTime = zapcore.ISO8601TimeEncoder

	config := zap.Config{
		Level:             lvl,
		Development:       dev,
		DisableCaller:     callerDisabled,
		DisableStacktrace: stacktraceDisabled,
		Sampling:          nil,
		Encoding:          encoding,
		EncoderConfig:     encoderCfg,
		OutputPaths: []string{
			"stdout",
		},
		ErrorOutputPaths: []string{
			"stderr",
		},
		InitialFields: map[string]interface{}{
			"pid": os.Getpid(),
		},
	}

	return zap.Must(config.Build())
}

func init() {
	//Init config
	if err := env.Parse(&settings); err != nil {
		fmt.Printf("[FATAL] Parse settings error: %s\n", err.Error())
		os.Exit(1)
	}
	fmt.Printf("%+v\n", settings)

	// Init zap logger
	logger := createLogger(settings.Development, settings.LogLevel)
	defer logger.Sync()

	zap.ReplaceGlobals(logger)
}

func main() {
	// Connect to Redis at start
	cache, err := data.NewRedisClient(settings.RedisAddr, settings.RedisDB)
	if err != nil {
		zap.S().Errorf("Failed to connect to Redis: %s", err.Error())
	}

	// Connect to MySQL at start
	mysql, err := data.NewMysqlClient(settings.MySQLHost, settings.MySQLUsername, settings.MySQLPassword, settings.MySQLDB)
	if err != nil {
		zap.S().Errorf("Failed to connect to MySQL: %s", err.Error())
	}

	embedFS, _ := fs.Sub(staticFiles, "static")

	fruit, err := fruitninja.NewFruitninja(&settings, cache, mysql, embedFS)
	if err != nil {
		fmt.Printf("[FATAL] Create Fruitninja error: %s\n", err.Error())
		os.Exit(1)
	}
	zap.S().Infof("Fruitninja runs in %s mode.", settings.Mode)

	// create http.Server using Echo as handler so we can call Shutdown
	srv := &http.Server{
		Addr:    settings.Listen,
		Handler: fruit.Server,
	}

	// start server in background
	srvErr := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			srvErr <- err
		} else {
			srvErr <- nil
		}
	}()

	// listen for termination signals
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	select {
	case sig := <-quit:
		zap.S().Infof("🛑🛑🛑 received signal %v, shutting down", sig)
	case err := <-srvErr:
		if err != nil {
			zap.S().Fatalf("server error: %v", err)
		}
		zap.S().Info("server stopped")
	}

	// give server time to shutdown gracefully
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(settings.GracefulShutdownTimeout)*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		zap.S().Errorf("error during server shutdown: %v", err)
	}

	// wait for active websockets to close (max 20s)
	if err := fruit.WaitForWebsockets(time.Duration(settings.GracefulShutdownTimeout) * time.Second); err != nil {
		zap.S().Warnf("websocket wait: %v", err)
	}

	zap.S().Info("shutdown complete")
}

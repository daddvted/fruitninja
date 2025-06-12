package main

import (
	"embed"
	"fmt"
	"io/fs"
	"os"

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
	fruit.Server.Logger.Fatal(fruit.Server.Start(settings.Listen))
}

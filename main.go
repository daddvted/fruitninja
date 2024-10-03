package main

import (
	"fmt"
	"os"

	"github.com/caarlos0/env/v9"
	"github.com/daddvted/fruitninja/data"
	"github.com/daddvted/fruitninja/fruitninja"

	// log "github.com/sirupsen/logrus"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var settings fruitninja.FruitNinjaSettings

func createLogger(dev bool, level string) *zap.Logger {
	enc := "json"
	if dev {
		enc = "console"
	}
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
		DisableCaller:     false,
		DisableStacktrace: false,
		Sampling:          nil,
		Encoding:          enc,
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
		fmt.Printf("Fatal parse settings: %s\n", err.Error())
		os.Exit(1)
	}
	fmt.Printf("%+v\n", settings)

	// Init zap logger
	logger := createLogger(settings.Development, settings.LogLevel)
	defer logger.Sync()

	zap.ReplaceGlobals(logger)
}

func main() {
	zap.S().Info("init-------------")
	zap.S().Debug("init-------------")
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

	httpSrv := fruitninja.FruitNinjaSetup(&settings, cache, mysql)
	zap.S().Infof("Fruitninja runs in %s mode.", settings.Mode)
	httpSrv.Logger.Fatal(httpSrv.Start(settings.Listen))
}

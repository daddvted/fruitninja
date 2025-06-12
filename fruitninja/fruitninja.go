package fruitninja

import (
	"io/fs"
	"net/http"

	"github.com/daddvted/fruitninja/data"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"go.uber.org/zap"
	"k8s.io/client-go/rest"
)

type FruitNinjaSettings struct {
	// Context    string `env:"NINJA_CONTEXT" envDefault:""`
	Mode          string `env:"NINJA_MODE" envDefault:"default"`
	Listen        string `env:"NINJA_LISTEN" envDefault:":8080"`
	Length        int    `env:"NINJA_JABBER_WORD" envDefault:"2"`
	Sleep         int    `env:"NINJA_SLEEP" envDefault:"0"`
	Name          string `env:"NINJA_NAME" envDefault:"default"`
	Count         int    `env:"NINJA_COUNT" envDefault:"1"`
	LogLevel      string `env:"NINJA_LOG_LEVEL" envDefault:"debug"`
	K8SAPI        string `env:"NINJA_K8A_API" envDefault:"https://kubernetes.default.svc"`
	Development   bool   `env:"NINJA_DEV" envDefault:"true"`
	K8SToken      string `env:"NINJA_K8S_TOKEN" envDefault:"eyJhbGciOiJSUzI1NiIsImtpZCI6InRBb1JyNzRaa3VYZmV6cmk4bHZybGJZcjVpOGN4cDhCSEtCdEJQMnp1RWMifQ.eyJpc3MiOiJrdWJlcm5ldGVzL3NlcnZpY2VhY2NvdW50Iiwia3ViZXJuZXRlcy5pby9zZXJ2aWNlYWNjb3VudC9uYW1lc3BhY2UiOiJkZXYiLCJrdWJlcm5ldGVzLmlvL3NlcnZpY2VhY2NvdW50L3NlY3JldC5uYW1lIjoiZGV2LWNvbnRhaW5lci1zZWNyZXQiLCJrdWJlcm5ldGVzLmlvL3NlcnZpY2VhY2NvdW50L3NlcnZpY2UtYWNjb3VudC5uYW1lIjoiZGV2LWNvbnRhaW5lci1zYSIsImt1YmVybmV0ZXMuaW8vc2VydmljZWFjY291bnQvc2VydmljZS1hY2NvdW50LnVpZCI6IjExMWY0OGRmLTFkYWEtNDljOS1hMzIzLTI0Nzc3ZWE0Y2U0ZCIsInN1YiI6InN5c3RlbTpzZXJ2aWNlYWNjb3VudDpkZXY6ZGV2LWNvbnRhaW5lci1zYSJ9.o0ZKu_ziOO3-GJ_kzYDnNq3UslhjRkue0TJWFAC9wgAgndhQi37r6-HwtMx3syHnC8Q5sNdG_Df0vYAKSH5PjgA2RqbMIoOWUwRxEDIwNBHHZ9xJrOu4gCZoxWqHgBskmjsqE5zVw5D6ksltAEZKFke15t2NlYuiiaz1Mj9mcEdUk7ryo5Z18VGKe6lsdbqfu_6GkUvN5NvzvoZcSrnc6VTGxuBV_c1Mfhk0lJpIlzEZjjDCpi6w-V3aH1oIJE5xmBxSOo9i8GRCV1SmEMsOErF9Qsc2QRwIiuIe4R4ALS-xSxqbrDBEAnI95feZDlsJU8yrqMsm0zxpkpHWSHQ13Q"`
	RedisAddr     string `env:"NINJA_REDIS_ADDR" envDefault:"localhost:6379"`
	RedisPassword string `env:"NINJA_REDIS_PSD" envDefault:""`
	RedisDB       int    `env:"NINJA_REDIS_DB" envDefault:"0"`
	MySQLHost     string `env:"NINJA_MYSQL_HOST" envDefault:"localhost:3306"`
	MySQLUsername string `env:"NINJA_MYSQL_USERNAME" envDefault:"root"`
	MySQLPassword string `env:"NINJA_MYSQL_PASSWORD" envDefault:"root"`
	MySQLDB       string `env:"NINJA_MYSQL_DB" envDefault:"fruit"`
}

var (
	k8sConfig *rest.Config

	fruitNinjaSettings *FruitNinjaSettings
	fruitNinjaCache    *data.Cache
	fruitNinjaMysql    *data.DB
)

type FruitNinja struct {
	settings FruitNinjaSettings
	Server   *echo.Echo
	k8s      *kubernetesMinion
	cache    *data.Cache
	db       *data.DB
}

func NewFruitninja(settings *FruitNinjaSettings, cache *data.Cache, db *data.DB, static fs.FS) (*FruitNinja, error) {
	fruitNinjaSettings = settings
	k8sminion, err := newKubernetesMinion(settings)
	if err != nil {
		return nil, err
	}

	fruitninja := &FruitNinja{
		settings: *settings,
		cache:    cache,
		db:       db,
		k8s:      k8sminion,
	}

	e := echo.New()

	// e.IPExtractor = echo.ExtractIPFromXFFHeader()
	e.IPExtractor = echo.ExtractIPFromRealIPHeader()

	// e.Use(middleware.Logger())
	// log.SetFormatter(&log.JSONFormatter{})
	// log.SetReportCaller(true)
	e.Use(middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogURI:       true,
		LogStatus:    true,
		LogLatency:   true,
		LogUserAgent: true,
		LogRequestID: true,
		LogValuesFunc: func(c echo.Context, v middleware.RequestLoggerValues) error {
			zap.L().Info("request",
				zap.String("URI", v.URI),
				zap.Int("status", v.Status),
				zap.Duration("latency", v.Latency),
				zap.String("reqid", v.RequestID),
				zap.String("useragent", v.UserAgent),
			)

			return nil
		},
	}))

	// "k8s" mode is used for service mesh demo
	if settings.Mode == "k8s" {
		e.GET("/*", fruitninja.getK8sBladeHandler)
		e.GET("/blade/:fruits", fruitninja.getK8sBladeHandler)

	} else {
		e.GET("/", fruitninja.indexHandler)
		e.GET("/hello", fruitninja.helloHandler)
		e.GET("/html/*", echo.WrapHandler(http.StripPrefix("/html/", http.FileServer(http.FS(static)))))
		e.GET("/ws", fruitninja.wsHandler)
		e.GET("/data", fruitninja.dataHandler)
	}
	fruitninja.Server = e
	return fruitninja, nil
}

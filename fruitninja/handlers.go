package fruitninja

import (
	"fmt"
	"html/template"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/daddvted/fruitninja/data"
	"github.com/labstack/echo/v5"
	"github.com/mileusna/useragent"
	"golang.org/x/net/websocket"

	"go.uber.org/zap"
)

type Template struct {
	templates *template.Template
}

func (t *Template) Render(w io.Writer, name string, data interface{}, c echo.Context) error {
	return t.templates.ExecuteTemplate(w, name, data)
}

func (fruitninja *FruitNinja) getK8sFruitHandler(c *echo.Context) error {
	zap.S().Infof("Request for [%s] service\n", fruitNinjaSettings.Name)
	url := c.Request().URL.Path
	zap.S().Debugf("Request URL: %s\n", url)

	if strings.TrimSpace(url) == "/" {
		// msg := strings.Repeat(fruitMap[fruitNinjaSettings.Name], fruitNinjaSettings.Count)
		msg := serveFruit(false, "")
		return c.String(http.StatusOK, fmt.Sprintf("%s\n", msg))
	}

	splitedURL := strings.SplitN(strings.Trim(url, "/"), "/", 2)
	serviceLength := len(splitedURL)
	zap.S().Debugf("Splited URL length: %d\n", serviceLength)

	skewer := []string{}
	// Append itself to skewer
	skewer = append(skewer, serveFruit(false, ""))
	ns := getNamespace()

	var urlRemainder, serviceURL string

	for serviceLength > 1 {
		nextService := splitedURL[0]
		if serviceLength > 1 {
			urlRemainder = splitedURL[1]
			serviceURL = "http://" + nextService + "." + ns + ".svc.cluster.local/" + urlRemainder
		} else {
			// If serviceLength == 1, no need to append urlRemainder
			serviceURL = "http://" + nextService + "." + ns + ".svc.cluster.local/"
		}
		zap.S().Debugf("Next service: %s\n", nextService)
		zap.S().Debugf("URL remainder: %s\n", urlRemainder)
		zap.S().Debugf("Next service url: %s\n", serviceURL)

		fruitEmoji, ok := getServingFruit(serviceURL)
		if ok {
			skewer = append(skewer, strings.TrimSpace(fruitEmoji))
			break
		} else {
			// Enclose fruit with square bracket,
			// when fruit emoji not return successfully.
			skewer = append(skewer, fmt.Sprintf("[%s]", nextService))
			splitedURL = strings.SplitN(urlRemainder, "/", 2)
			fmt.Printf("splited url: %+v\n", splitedURL)
			serviceLength = len(splitedURL)
			fmt.Printf("service LENGTH: %d\n", serviceLength)
		}

	}

	if serviceLength == 1 {
		nextService := splitedURL[0]
		serviceURL := "http://" + nextService + "." + ns + ".svc.cluster.local/"
		fmt.Printf("next service url: %s\n", serviceURL)
		fruitEmoji, ok := getServingFruit(serviceURL)
		if ok {
			// skewer = append(skewer, strings.TrimSpace(fruitEmoji))
			fmt.Println(fruitEmoji)
			skewer = append(skewer, strings.TrimSpace(fruitEmoji))
		} else {
			// Enclose fruit with square bracket,
			// when fruit emoji not return successfully.
			skewer = append(skewer, fmt.Sprintf("[%s]", nextService))
		}

	}
	bladeString := strings.Join(skewer, "->")
	// return c.String(http.StatusOK, strings.Join(skewer, "->"))
	return c.String(http.StatusOK, fmt.Sprintf("%s\n", bladeString))
}

func (fruitninja *FruitNinja) getK8sBladeHandler(c *echo.Context) error {
	url := c.Request().URL.Path
	zap.S().Debugf("Request URL: %s\n", url)
	queryStr := c.Param("fruits")
	zap.S().Debugf("Query string: %s\n", queryStr)

	splitedURL := strings.SplitN(strings.Trim(queryStr, "/"), "/", 2)
	zap.S().Debugf("Splited URL: %q\n", splitedURL)
	fruitNO := len(splitedURL)
	zap.S().Debugf("No of fruits: %d\n", fruitNO)
	ns := getNamespace()

	var serviceURL string

	if fruitNO > 1 {
		serviceURL = "http://" + splitedURL[0] + "." + ns + ".svc.cluster.local/" + splitedURL[1]
	} else {
		// If serviceLength == 1, no need to append urlRemainder
		serviceURL = "http://" + splitedURL[0] + "." + ns + ".svc.cluster.local/"
	}
	fruitEmoji, ok := getServingFruit(serviceURL)
	if ok {
		return c.String(http.StatusOK, fruitEmoji)
	} else {
		return c.String(400, fmt.Sprintf("%s\n", serveFruit(false, "blade")))
	}
}

func (fruitninja *FruitNinja) dataHandler(c *echo.Context) error {
	var jabberText string
	var cacheText string
	var dbText string
	var respText string

	fmt.Println(c.Request().UserAgent())
	ua := useragent.Parse(c.Request().UserAgent())

	fruitName := serveFruit(true, "")

	// Cache
	if fruitNinjaCache == nil {
		// Try to connect Redis again.
		redis, err := data.NewRedisClient(fruitNinjaSettings.RedisAddr, fruitNinjaSettings.RedisDB)
		if err != nil {
			zap.S().Errorf("Failed to connect to Redis: %s", err.Error())
			cacheText = data.CacheErrorText
		} else {
			fruitNinjaCache = redis
			fruitNinjaCache.AppendKey("fruitninja", fruitName)
			cacheText = fruitNinjaCache.GetKey("fruitninja")
		}
	} else {
		fruitNinjaCache.AppendKey("fruitninja", fruitName)
		cacheText = fruitNinjaCache.GetKey("fruitninja")
	}
	zap.S().Debug(fruitNinjaMysql)

	// DB
	if fruitNinjaMysql != nil {
		_, err := fruitNinjaMysql.GetSingleFruit(fruitName)
		if err != nil {
			zap.S().Error(err)
			_, err := fruitNinjaMysql.AddFruit(fruitName)
			if err != nil {
				zap.S().Error(err)
			}
		} else {
			err := fruitNinjaMysql.AddAmount(fruitName)
			if err != nil {
				zap.S().Error(err)
			}
		}

		// Query all fruits
		fruits, err := fruitNinjaMysql.GetFruits()
		if err != nil {
			zap.S().Error(err)
			dbText = err.Error()
		} else {
			var text []string
			for _, fruit := range fruits {
				text = append(text, fmt.Sprintf("%s: %d", fruit.Name, fruit.Amount))
			}
			dbText = fmt.Sprintf("\n%s", strings.Join(text, "\n"))
		}

	} else {
		// re-connect
		mysql, err := data.NewMysqlClient(fruitNinjaSettings.MySQLHost, fruitNinjaSettings.MySQLUsername, fruitNinjaSettings.MySQLPassword, fruitNinjaSettings.MySQLDB)
		if err != nil {
			zap.S().Errorf("Failed to connect to MySQL again: %s", err.Error())
			dbText = "Failed to connect to MySQL"
		}
		fruitNinjaMysql = mysql
	}

	jabberText = fmt.Sprintf("%s: %s", generatePetName(true), strings.Repeat(fruitName, fruitNinjaSettings.Count))

	if ua.Name == "curl" {
		version := fmt.Sprintf("--- Version: %s ---", Version)
		respText = fmt.Sprintf("%s\n%s\n[CACHE]: %s\n[DB]: %s\n", version, jabberText, cacheText, dbText)
		return c.String(http.StatusOK, respText)
	} else {
		version := fmt.Sprintf("<h1>Version: %s</h1>", Version)
		respText = fmt.Sprintf("%s\n%s<br/>[CACHE]: %s<br/>[DB]: <br/>%s<br/>", version, jabberText, cacheText, dbText)
		return c.HTML(http.StatusOK, respText)
	}
}

func (fruitninja *FruitNinja) helloHandler(c *echo.Context) error {
	ua_text := c.Request().Header.Get("User-Agent")
	zap.S().Debugf("User-agent: %s\n", ua_text)
	ua := useragent.Parse(ua_text)

	fruit := serveFruit(false, "")
	serverIP := getOutboundIP()
	hostname := getHostname()

	clientIP := c.RealIP()
	if ua.IsUnknown() {
		resp := fmt.Sprintf("Version: \t%s\nClient IP: \t%s\n%s@%s\n", Version, clientIP, fruit, hostname)
		return c.String(http.StatusOK, resp)

	} else {
		resp := fmt.Sprintf("<p>Version: %s</p><p>Client IP: %s</p><p><span style='font-size: 30px;'>%s@%s(%s)</span></p>", Version, clientIP, fruit, hostname, serverIP)
		return c.HTML(http.StatusOK, resp)
	}
}

func (fruitninja *FruitNinja) indexHandler(c *echo.Context) error {
	sleep := fruitninja.settings.Sleep
	if sleep > 0 {
		time.Sleep(time.Duration(sleep) * time.Second)
	}
	ua_text := c.Request().Header.Get("User-Agent")
	ua := useragent.Parse(ua_text)
	hostname := getHostname()
	fruit := serveFruit(false, "")

	if ua.IsUnknown() {
		resp := fmt.Sprintf("%s@%s\n", fruit, hostname)
		return c.String(http.StatusOK, resp)

	} else {
		resp := fmt.Sprintf("<p><span style='font-size: 30px;'>%s@%s</span></p>", fruit, hostname)
		return c.HTML(http.StatusOK, resp)
	}
}

func (fruitninja *FruitNinja) wsHandler(c *echo.Context) error {
	websocket.Handler(func(ws *websocket.Conn) {
		defer ws.Close()
		msg := "Welcome to FruitNinja"
		for {
			// Write
			zap.S().Infof("🛒 send %s to client", msg)
			err := websocket.Message.Send(ws, msg)
			if err != nil {
				zap.S().Error(err)
				break
			}

			// Read
			err = websocket.Message.Receive(ws, &msg)
			if err != nil {
				zap.S().Info("🐞 connection closed")
				zap.S().Error(err)
				break
			}
			zap.S().Infof("🐛 received %s from client", msg)
			if isNumeric(msg) {
				msg = serveFruit(false, "")
			} else {
				msg = serveFruit(false, msg)
			}
		}
	}).ServeHTTP(c.Response(), c.Request())
	return nil
}

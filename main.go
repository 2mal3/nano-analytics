package main

import (
	"cmp"
	"crypto/md5"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/go-co-op/gocron/v2"
	"github.com/joho/godotenv"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/mileusna/useragent"
	"github.com/oschwald/geoip2-golang"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type Hit struct {
	Ip       string `gorm:"primaryKey"`
	Path     string `gorm:"primaryKey"`
	Action   string `gorm:"primaryKey"`
	Date     string `gorm:"primaryKey"`
	Country  string
	Device   string
	Browser  string
	Referrer string
}

type Date struct {
	Date string `gorm:"primaryKey"`
}

var db *gorm.DB
var geolite *geoip2.Reader
var adminPasswordHash string
var adminUsername string

func main() {
	var err error

	// Load environment variables
	godotenv.Load()
	adminPasswordHash = os.Getenv("ADMIN_PASSWORD_HASH")
	if adminPasswordHash == "" {
		fmt.Println("ADMIN_PASSWORD_HASH not set")
		os.Exit(1)
	}
	adminUsername = os.Getenv("ADMIN_USERNAME")
	if adminUsername == "" {
		adminUsername = "admin"
	}

	// Connect to the sql database
	db, err = gorm.Open(sqlite.Open("database/hits.db"), &gorm.Config{
		TranslateError: true,
		Logger:         logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		fmt.Println("Failed to open database: " + err.Error())
		os.Exit(1)
	}
	if err = db.AutoMigrate(&Hit{}, &Date{}); err != nil {
		panic("failed to migrate database: " + err.Error())
	}

	createDatabaseDate()

	// Connect to the golite2 database
	geolite, err = geoip2.Open("GeoLite2-Country.mmdb")
	if err != nil {
		panic("failed to open geolite database: " + err.Error())
	}
	defer geolite.Close()

	// Start cronjobs
	s, err := gocron.NewScheduler()
	if err != nil {
		panic(err)
	}
	_, err = s.NewJob(gocron.CronJob("0 0 * * *", false), gocron.NewTask(createDatabaseDate))
	if err != nil {
		panic(err)
	}
	s.Start()

	// Start echo server
	e := echo.New()

	e.Logger.SetLevel(2)
	e.Logger.SetHeader("[${time_rfc3339}] [${short_file}/${level}]:")

	e.IPExtractor = echo.ExtractIPFromXFFHeader()

	e.GET("/track/:path", track)

	// Special password protected routes
	e.GET("", func(ctx echo.Context) error { return ctx.Redirect(http.StatusSeeOther, "/stats") })
	statRoutes := e.Group("/stats")
	statRoutes.Use(middleware.BasicAuth(func(username string, password string, c echo.Context) (bool, error) {
		if subtle.ConstantTimeCompare([]byte(username), []byte(adminUsername)) == 1 &&
			verifyPassword(password, adminPasswordHash) {
			return true, nil
		}
		return false, nil
	}))

	statRoutes.GET("", statsOverviewRoute)
	statRoutes.GET("/:path", statsRoute)

	e.Logger.Fatal(e.Start(":1323"))
}

func verifyPassword(password string, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// Creates the current date in the database each day for the data view to work properly
func createDatabaseDate() {
	today := time.Now().Format("2006-01-02")
	dateEntry := Date{
		Date: today,
	}
	db.Create(&dateEntry)
}

func track(ctx echo.Context) error {
	ua := useragent.Parse(ctx.Request().UserAgent())
	if ua.Bot {
		return ctx.NoContent(http.StatusOK)
	}

	today := time.Now().Format("2006-01-02")

	ip := ctx.RealIP()
	ipHash := md5.Sum([]byte(ip + today))
	ipHashString := hex.EncodeToString(ipHash[:])

	geoResponse, err := geolite.Country(net.ParseIP(ip))
	if err != nil {
		ctx.Logger().Error(err)
		return ctx.NoContent(http.StatusOK)
	}
	countryName := geoResponse.Country.Names["en"]

	path := ctx.Param("path")

	action := ctx.QueryParam("action")

	rawReferrer := ctx.QueryParam("referrer")
	parsedUrl, err := url.Parse(rawReferrer)
	if err != nil {
		ctx.Logger().Error(err)
		return ctx.NoContent(http.StatusOK)
	}
	rawReferrerDomain := parsedUrl.Hostname()
	referrerDomain := strings.TrimPrefix(rawReferrerDomain, "www.")

	hit := Hit{
		Ip:       ipHashString,
		Path:     path,
		Action:   action,
		Date:     today,
		Country:  countryName,
		Device:   ua.OS,
		Browser:  ua.Name,
		Referrer: referrerDomain,
	}
	if result := db.Create(&hit); result.Error != nil && !errors.Is(result.Error, gorm.ErrDuplicatedKey) {
		ctx.Logger().Error(result.Error)
		return ctx.NoContent(http.StatusOK)
	}

	ctx.Response().Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, post-check=0, pre-check=0")
	return ctx.NoContent(http.StatusOK)
}

func statsOverviewRoute(ctx echo.Context) error {
	var hits []Hit
	result := db.Select("path").Distinct("path").Find(&hits)
	if result.Error != nil {
		ctx.Logger().Error(result.Error)
	}

	return statsOverviewTempl(hits).Render(ctx.Request().Context(), ctx.Response().Writer)
}

type stat struct {
	Name  string
	Count int
}

func statsRoute(ctx echo.Context) error {
	monthBefore := time.Now().AddDate(0, 0, -30).Format("2006-01-02")

	path := ctx.Param("path")

	var views []stat
	db.Raw("SELECT d.date AS name, COUNT(h.ip) AS count FROM dates AS d LEFT JOIN (SELECT * FROM hits WHERE path = ?) AS h ON h.date = d.date GROUP BY d.date HAVING d.date >= ?", path, monthBefore).Scan(&views)

	var actions []stat
	db.Raw("SELECT action AS name, COUNT(*) AS count FROM hits WHERE path = ? GROUP BY action HAVING date >= ?", path, monthBefore).Scan(&actions)

	var countries []stat
	db.Raw("SELECT country AS name, COUNT(*) AS count FROM hits WHERE path = ? AND date >= ? GROUP BY country", path, monthBefore).Scan(&countries)
	slices.SortFunc(countries, sortSats)

	var browsers []stat
	db.Raw("SELECT browser AS name, COUNT(*) AS count FROM hits WHERE path = ? AND date >= ? GROUP BY browser", path, monthBefore).Scan(&browsers)
	slices.SortFunc(browsers, sortSats)

	var devices []stat
	db.Raw("SELECT device AS name, COUNT(*) AS count FROM hits WHERE path = ? AND date >= ? GROUP BY device", path, monthBefore).Scan(&devices)
	slices.SortFunc(devices, sortSats)

	var referrers []stat
	db.Raw("SELECT referrer AS name, COUNT(*) AS count FROM hits WHERE path = ? AND date >= ? GROUP BY referrer", path, monthBefore).Scan(&referrers)
	slices.SortFunc(referrers, sortSats)

	return statsTempl(path, views, actions, countries, browsers, devices, referrers).Render(ctx.Request().Context(), ctx.Response().Writer)
}

func sortSats(a, b stat) int {
	return cmp.Compare(b.Count, a.Count)
}

package main

import (
	"os"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func main() {
	thornodeAPI := os.Getenv("THORNODE_API")
	if thornodeAPI == "" {
		thornodeAPI = "https://gateway.liquify.com/chain/thorchain_api/"
	}
	InitCache(thornodeAPI)
	initMidgardDB()

	// initialize http server and slug routing
	e := echo.New()
	e.Use(middleware.Gzip())

	// geckoterminal api endpoints
	e.GET("/thorchain/geckoterminal/latest-block", GeckoterminalLatestBlock)
	e.GET("/thorchain/geckoterminal/asset", GeckoterminalAsset)
	e.GET("/thorchain/geckoterminal/pair", GeckoterminalPair)
	e.GET("/thorchain/geckoterminal/events", GeckoterminalEvents)

	e.Logger.Fatal(e.Start(":1323"))
}

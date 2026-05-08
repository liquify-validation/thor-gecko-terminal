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
	midgardAPI := os.Getenv("MIDGARD_API")
	if midgardAPI == "" {
		midgardAPI = "https://gateway.liquify.com/chain/thorchain_midgard"
	}

	InitCache(thornodeAPI, midgardAPI)
	initMidgardDB()

	e := echo.New()
	e.Use(middleware.Gzip())

	// API documentation
	e.GET("/openapi.yaml", OpenAPISpecHandler)
	e.GET("/docs", SwaggerUIHandler)

	// GeckoTerminal endpoints
	e.GET("/thorchain/geckoterminal/latest-block", GeckoterminalLatestBlock)
	e.GET("/thorchain/geckoterminal/asset", GeckoterminalAsset)
	e.GET("/thorchain/geckoterminal/pair", GeckoterminalPair)
	e.GET("/thorchain/geckoterminal/events", GeckoterminalEvents)

	// CoinMarketCap endpoints
	e.GET("/thorchain/cmc/summary", CMCSummary)
	e.GET("/thorchain/cmc/assets", CMCAssets)
	e.GET("/thorchain/cmc/ticker", CMCTicker)
	e.GET("/thorchain/cmc/trades", CMCTrades)
	e.GET("/thorchain/cmc/swaps", CMCSwaps)
	e.GET("/thorchain/cmc/orderbook", CMCOrderbookHandler)
	e.GET("/thorchain/cmc/proof-of-reserves", CMCProofOfReservesHandler)
	e.GET("/thorchain/cmc/proof-of-liabilities", CMCProofOfLiabilitiesHandler)

	e.Logger.Fatal(e.Start(":1323"))
}

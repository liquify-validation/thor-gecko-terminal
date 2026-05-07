// CoinMarketCap (CMC) Integration API - HTTP handlers.

package main

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	echo "github.com/labstack/echo/v4"
	"github.com/rs/zerolog/log"
)

// CMCSummary returns an overview of all RUNE-paired markets with 24h stats.
//
//	GET /thorchain/cmc/summary
func CMCSummary(c echo.Context) error {
	pools := CachedMidgardPools()
	if len(pools) == 0 {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{
			"error": "midgard pool data not available",
		})
	}

	pairIDs := buildPairIDMap(pools)
	summary := make([]CMCSummaryEntry, 0, len(pools))

	for _, p := range pools {
		if !strings.EqualFold(p.Status, "available") {
			continue
		}

		assetPrice := parseFloatOrZero(p.AssetPrice) // RUNE per asset
		if assetPrice <= 0 {
			continue
		}

		// trading_pair format: RUNE_<symbol>; base = RUNE, quote = asset.
		// last_price is the price of base (RUNE) denominated in quote (asset),
		// i.e. how much asset you get per 1 RUNE = 1 / assetPrice.
		runePriceInAsset := 1.0 / assetPrice

		baseVolume := e8ToFloat(p.Volume24h)             // 24h volume in RUNE
		quoteVolume := baseVolume * runePriceInAsset     // converted to asset
		_, quoteSymbol := extractChainSymbol(p.Asset)

		summary = append(summary, CMCSummaryEntry{
			TradingPairs:    pairIDs[p.Asset],
			BaseCurrency:    "RUNE",
			QuoteCurrency:   quoteSymbol,
			LastPrice:       runePriceInAsset,
			LowestAsk:       runePriceInAsset, // AMM has no spread
			HighestBid:      runePriceInAsset, // AMM has no spread
			BaseVolume:      baseVolume,
			QuoteVolume:     quoteVolume,
			HighestPrice24h: runePriceInAsset, // 24h high not in /v2/pools
			LowestPrice24h:  runePriceInAsset, // 24h low not in /v2/pools
		})
	}

	return c.JSON(http.StatusOK, summary)
}

// CMCAssets returns details for every asset traded in a RUNE pool, plus RUNE itself.
//
//	GET /thorchain/cmc/assets
func CMCAssets(c echo.Context) error {
	pools := CachedMidgardPools()
	if len(pools) == 0 {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{
			"error": "midgard pool data not available",
		})
	}

	assets := make(map[string]CMCAsset, len(pools)+1)

	// RUNE itself (no contract address, no pool entry).
	assets["RUNE"] = CMCAsset{
		Name:        "THORChain",
		CanWithdraw: "true",
		CanDeposit:  "true",
		MinWithdraw: "0",
		MaxWithdraw: "0",
		MakerFee:    "0",
		TakerFee:    "0",
	}

	pairIDs := buildPairIDMap(pools)

	for _, p := range pools {
		canTrade := strings.EqualFold(p.Status, "available")
		chain, _ := extractChainSymbol(p.Asset)
		contract := extractContractAddress(p.Asset)

		// Use the pair ID's quote portion as the asset key so collisions
		// (e.g. USDC on multiple chains) remain unique in the map.
		key := strings.TrimPrefix(pairIDs[p.Asset], "RUNE_")

		name := p.Asset
		if friendly, ok := assetNames[p.Asset]; ok {
			name = friendly
		}

		assets[key] = CMCAsset{
			Name:               name,
			CanWithdraw:        boolToString(canTrade),
			CanDeposit:         boolToString(canTrade),
			MinWithdraw:        "0",
			MaxWithdraw:        "0",
			MakerFee:           "0",
			TakerFee:           "0",
			ContractAddress:    contract,
			ContractAddressURL: contractAddressURL(chain, contract),
		}
	}

	return c.JSON(http.StatusOK, assets)
}

// CMCTicker returns 24h pricing/volume for every market keyed by trading pair.
//
//	GET /thorchain/cmc/ticker
func CMCTicker(c echo.Context) error {
	pools := CachedMidgardPools()
	if len(pools) == 0 {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{
			"error": "midgard pool data not available",
		})
	}

	pairIDs := buildPairIDMap(pools)
	tickers := make(map[string]CMCTickerEntry, len(pools))

	for _, p := range pools {
		if !strings.EqualFold(p.Status, "available") {
			continue
		}

		assetPrice := parseFloatOrZero(p.AssetPrice)
		if assetPrice <= 0 {
			continue
		}

		runePriceInAsset := 1.0 / assetPrice
		baseVolume := e8ToFloat(p.Volume24h)
		quoteVolume := baseVolume * runePriceInAsset

		tickers[pairIDs[p.Asset]] = CMCTickerEntry{
			LastPrice:   runePriceInAsset,
			BaseVolume:  baseVolume,
			QuoteVolume: quoteVolume,
			IsFrozen:    0,
		}
	}

	return c.JSON(http.StatusOK, tickers)
}

// CMCTrades returns recent swap trades for a given market pair.
//
//	GET /thorchain/cmc/trades?market_pair=RUNE_BTC
func CMCTrades(c echo.Context) error {
	marketPair := c.QueryParam("market_pair")
	if marketPair == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "market_pair parameter is required",
		})
	}

	if midgardDB == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{
			"error": "midgard database not available",
		})
	}

	pools := CachedMidgardPools()
	pairIDs := buildPairIDMap(pools)
	asset := findAssetByPairID(marketPair, pairIDs)
	if asset == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "unknown market_pair: " + marketPair,
		})
	}

	ctx, cancel := context.WithTimeout(c.Request().Context(), 30*time.Second)
	defer cancel()

	trades, err := queryRecentTrades(ctx, asset, 100)
	if err != nil {
		log.Error().Err(err).Str("asset", asset).Msg("failed to query trades")
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "failed to fetch trades",
		})
	}

	return c.JSON(http.StatusOK, trades)
}

// CMCSwaps returns recent swaps in DEX section C2 (subgraph-style) format.
//
//	GET /thorchain/cmc/swaps?limit=100
func CMCSwaps(c echo.Context) error {
	if midgardDB == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{
			"error": "midgard database not available",
		})
	}

	limit := 100
	if l := c.QueryParam("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 1000 {
			limit = parsed
		}
	}

	ctx, cancel := context.WithTimeout(c.Request().Context(), 30*time.Second)
	defer cancel()

	swaps, err := queryRecentSwaps(ctx, limit)
	if err != nil {
		log.Error().Err(err).Msg("failed to query swaps")
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "failed to fetch swaps",
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{"swaps": swaps})
}

func boolToString(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

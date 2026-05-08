// CoinMarketCap (CMC) Orderbook handler.
//
// THORChain has no real orderbook — it's a constant-product AMM. This file
// synthesizes a level-2 orderbook from the bonding curve by discretizing it
// into price levels at fixed percentage steps from spot. Larger trades incur
// worse prices on an AMM, so each ask/bid level represents the incremental
// quantity available between two consecutive price points.
//
// For pool x = RUNE depth, y = asset depth, k = x*y, spot price P = y/x:
//
//	Asks (buying base from pool, price > spot):
//	  cumulative ΔRUNE to reach price P' = x − √(k/P')
//
//	Bids (selling base to pool, price < spot):
//	  cumulative ΔRUNE the pool will absorb to reach P' = √(k/P') − x
//
// Slip-based fees are NOT applied to these prices — the response shows the
// pure curve. Real trade prices will be slightly worse due to slip fees,
// which are surfaced separately via /pair.feeBps.

package main

import (
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	echo "github.com/labstack/echo/v4"
)

const (
	defaultOrderbookLevels = 50
	maxOrderbookLevels     = 500
	defaultLevelStep       = 0.001 // 0.1% per level
)

// CMCOrderbookHandler returns a synthesized AMM orderbook for a market pair.
//
//	GET /thorchain/cmc/orderbook?market_pair=RUNE_BTC&depth=100&level=2
//
// Query params:
//   - market_pair (required): e.g. "RUNE_BTC"
//   - depth (optional): one of [0, 5, 10, 20, 50, 100, 500]. Total entries
//     across both sides; 0 means use the default depth. Defaults to 100.
//   - level (optional): 1 = best bid/ask only, 2 = arranged levels (default),
//     3 = deeper book (up to maxOrderbookLevels per side).
func CMCOrderbookHandler(c echo.Context) error {
	marketPair := c.QueryParam("market_pair")
	if marketPair == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "market_pair parameter is required",
		})
	}

	pools := CachedMidgardPools()
	if len(pools) == 0 {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{
			"error": "midgard pool data not available",
		})
	}

	pairIDs := buildPairIDMap(pools)
	asset := findAssetByPairID(marketPair, pairIDs)
	if asset == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "unknown market_pair: " + marketPair,
		})
	}

	pool, ok := findMidgardPool(pools, asset)
	if !ok || !strings.EqualFold(pool.Status, "available") {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{
			"error": "pool not available: " + asset,
		})
	}

	runeDepth := e8ToFloat(pool.RuneDepth)
	assetDepth := e8ToFloat(pool.AssetDepth)
	if runeDepth <= 0 || assetDepth <= 0 {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{
			"error": "pool has no liquidity",
		})
	}

	levelsPerSide := resolveLevels(c.QueryParam("level"), c.QueryParam("depth"))
	bids, asks := synthesizeOrderbook(runeDepth, assetDepth, levelsPerSide, defaultLevelStep)

	return c.JSON(http.StatusOK, CMCOrderbook{
		Timestamp: time.Now().UnixMilli(),
		Bids:      bids,
		Asks:      asks,
	})
}

// findMidgardPool returns the pool entry for the given asset, or false if missing.
func findMidgardPool(pools []MidgardPool, asset string) (MidgardPool, bool) {
	for _, p := range pools {
		if p.Asset == asset {
			return p, true
		}
	}
	return MidgardPool{}, false
}

// resolveLevels picks how many levels to generate per side based on the
// `level` and `depth` query parameters.
func resolveLevels(levelStr, depthStr string) int {
	// `level` takes precedence: 1 = top of book, 3 = deep.
	if level, err := strconv.Atoi(levelStr); err == nil {
		switch level {
		case 1:
			return 1
		case 3:
			return maxOrderbookLevels
		}
	}

	// `depth` is total entries across both sides; halve it for per-side count.
	if depth, err := strconv.Atoi(depthStr); err == nil && depth > 0 {
		perSide := depth / 2
		if perSide > maxOrderbookLevels {
			perSide = maxOrderbookLevels
		}
		if perSide < 1 {
			perSide = 1
		}
		return perSide
	}

	return defaultOrderbookLevels
}

// synthesizeOrderbook discretizes a constant-product AMM curve into levels.
//
// runeDepth and assetDepth are in human-decimal units (already divided by 1e8).
// stepPct is the relative price increment per level (e.g. 0.001 = 0.1%).
//
// Returns bids (descending price) and asks (ascending price), each entry
// [price, quantity] where price is asset-per-RUNE and quantity is RUNE.
func synthesizeOrderbook(runeDepth, assetDepth float64, levels int, stepPct float64) (bids, asks [][2]float64) {
	if runeDepth <= 0 || assetDepth <= 0 || levels <= 0 || stepPct <= 0 {
		return nil, nil
	}

	x := runeDepth
	y := assetDepth
	k := x * y
	spot := y / x

	asks = make([][2]float64, 0, levels)
	prevX := x
	for i := 1; i <= levels; i++ {
		targetPrice := spot * (1 + float64(i)*stepPct)
		newX := math.Sqrt(k / targetPrice)
		runeAtThisLevel := prevX - newX
		if runeAtThisLevel <= 0 {
			break
		}
		asks = append(asks, [2]float64{targetPrice, runeAtThisLevel})
		prevX = newX
	}

	bids = make([][2]float64, 0, levels)
	prevX = x
	for i := 1; i <= levels; i++ {
		targetPrice := spot * (1 - float64(i)*stepPct)
		if targetPrice <= 0 {
			break
		}
		newX := math.Sqrt(k / targetPrice)
		runeAtThisLevel := newX - prevX
		if runeAtThisLevel <= 0 {
			break
		}
		bids = append(bids, [2]float64{targetPrice, runeAtThisLevel})
		prevX = newX
	}

	return bids, asks
}

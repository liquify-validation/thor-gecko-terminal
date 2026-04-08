package main

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	echo "github.com/labstack/echo/v4"
	"github.com/rs/zerolog/log"
	openapi "gitlab.com/thorchain/thornode/v3/openapi/gen"
)

func GeckoterminalLatestBlock(c echo.Context) error {
	start := time.Now()
	log.Info().Msg("geckoterminal latest-block request")

	ctx, cancel := context.WithTimeout(c.Request().Context(), 30*time.Second)
	defer cancel()

	if midgardDB == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{
			"error": "midgard database not available",
		})
	}

	// Get latest indexed block from Midgard instead of THORNode.
	// This ensures /events will always have data for the returned block.
	query := `SELECT height, timestamp FROM midgard.block_log ORDER BY height DESC LIMIT 1`

	var blockHeight, blockTimestamp int64
	err := midgardDB.QueryRowContext(ctx, query).Scan(&blockHeight, &blockTimestamp)
	if err != nil {
		log.Error().Err(err).Msg("failed to get latest block from midgard")
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "failed to fetch latest block",
		})
	}

	response := LatestBlockResponse{
		Block: Block{
			BlockNumber:    blockHeight,
			BlockTimestamp: blockTimestamp / 1_000_000_000, // nanoseconds to seconds
		},
	}

	log.Info().
		Int64("blockNumber", blockHeight).
		Dur("duration", time.Since(start)).
		Msg("geckoterminal latest-block completed")

	return c.JSON(http.StatusOK, response)
}

func GeckoterminalAsset(c echo.Context) error {
	start := time.Now()
	originalAssetID := c.QueryParam("id")
	if originalAssetID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "asset id parameter is required",
		})
	}

	assetID := strings.ToUpper(originalAssetID)
	log.Info().
		Str("originalAssetID", originalAssetID).
		Str("normalizedAssetID", assetID).
		Msg("geckoterminal asset request")

	pools := CachedThornodeThorchainPools()
	if len(pools) == 0 {
		log.Error().Msg("pools cache is empty")
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "pools data not available",
		})
	}

	targetPool := findPool(pools, assetID)

	// THOR.RUNE doesn't have its own pool — return synthetic response.
	if targetPool == nil && assetID == "THOR.RUNE" {
		coinGeckoID := "thorchain"
		return c.JSON(http.StatusOK, AssetResponse{
			Asset: Asset{
				ID:          "THOR.RUNE",
				Name:        "THORChain",
				Symbol:      "RUNE",
				Decimals:    8,
				CoinGeckoID: &coinGeckoID,
			},
		})
	}

	// Unknown pool — return basic info extracted from the asset ID.
	if targetPool == nil {
		log.Warn().Str("assetID", assetID).Msg("pool not found - returning basic asset info")
		name, symbol := parseAssetID(assetID)
		return c.JSON(http.StatusOK, AssetResponse{
			Asset: Asset{
				ID:       assetID,
				Name:     name,
				Symbol:   symbol,
				Decimals: 8,
			},
		})
	}

	if targetPool.Status != "Available" {
		log.Info().Str("assetID", assetID).Str("status", targetPool.Status).
			Msg("pool not available - returning info with status")
	}

	name, symbol := parseAssetID(assetID)
	if fullName, ok := assetNames[assetID]; ok {
		name = fullName
	}

	var coinGeckoID *string
	if cgID, ok := coinGeckoMapping[assetID]; ok && cgID != "" {
		coinGeckoID = &cgID
	}

	// Populate supply from pool balance (decimalized per spec).
	var totalSupply, circulatingSupply *string
	if targetPool.BalanceAsset != "" {
		balE8, err := strconv.ParseInt(targetPool.BalanceAsset, 10, 64)
		if err == nil && balE8 > 0 {
			supply := fmt.Sprintf("%.8f", float64(balE8)/1e8)
			totalSupply = &supply
			circulatingSupply = &supply
		}
	}

	log.Info().Str("assetID", assetID).Str("status", targetPool.Status).
		Dur("duration", time.Since(start)).Msg("geckoterminal asset completed")

	return c.JSON(http.StatusOK, AssetResponse{
		Asset: Asset{
			ID:                assetID,
			Name:              name,
			Symbol:            symbol,
			Decimals:          8,
			TotalSupply:       totalSupply,
			CirculatingSupply: circulatingSupply,
			CoinGeckoID:       coinGeckoID,
		},
	})
}

func GeckoterminalPair(c echo.Context) error {
	start := time.Now()
	originalPairID := c.QueryParam("id")
	if originalPairID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "pair id parameter is required",
		})
	}

	pairID := strings.ToUpper(originalPairID)
	log.Info().
		Str("originalPairID", originalPairID).
		Str("normalizedPairID", pairID).
		Msg("geckoterminal pair request")

	pools := CachedThornodeThorchainPools()
	if len(pools) == 0 {
		log.Error().Msg("pools cache is empty")
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "pools data not available",
		})
	}

	targetPool := findPool(pools, pairID)

	if targetPool == nil {
		log.Warn().Str("pairID", pairID).Msg("pool not found - returning basic pair info")
		return c.JSON(http.StatusOK, PairResponse{
			Pair: Pair{
				ID:       pairID,
				DexKey:   "thorchain",
				Asset0ID: "THOR.RUNE",
				Asset1ID: pairID,
			},
		})
	}

	if targetPool.Status != "Available" {
		log.Info().Str("pairID", pairID).Str("status", targetPool.Status).
			Msg("pool not available - returning info with status")
	}

	feeBps := getMimirFee()

	log.Info().Str("pairID", pairID).Str("status", targetPool.Status).
		Dur("duration", time.Since(start)).Msg("geckoterminal pair completed")

	return c.JSON(http.StatusOK, PairResponse{
		Pair: Pair{
			ID:       pairID,
			DexKey:   "thorchain",
			Asset0ID: "THOR.RUNE",
			Asset1ID: pairID,
			FeeBps:   feeBps,
		},
	})
}

func GeckoterminalEvents(c echo.Context) error {
	start := time.Now()

	fromBlockStr := c.QueryParam("fromBlock")
	toBlockStr := c.QueryParam("toBlock")
	if fromBlockStr == "" || toBlockStr == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "fromBlock and toBlock parameters are required",
		})
	}

	fromBlock, err := strconv.ParseInt(fromBlockStr, 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "invalid fromBlock parameter",
		})
	}

	toBlock, err := strconv.ParseInt(toBlockStr, 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "invalid toBlock parameter",
		})
	}

	if fromBlock > toBlock {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "fromBlock must be <= toBlock",
		})
	}

	blockRange := toBlock - fromBlock
	if blockRange > 250 {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "block range too large - maximum 250 blocks per request (recommended: 100 blocks)",
		})
	}

	log.Info().Int64("fromBlock", fromBlock).Int64("toBlock", toBlock).
		Int64("blockRange", blockRange).Msg("geckoterminal events request")

	if midgardDB == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{
			"error": "midgard database not available",
		})
	}

	ctx, cancel := context.WithTimeout(c.Request().Context(), 120*time.Second)
	defer cancel()

	events, err := queryEvents(ctx, fromBlock, toBlock)
	if err != nil {
		log.Error().Err(err).Msg("failed to query events")
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "failed to fetch event data",
		})
	}

	log.Info().Int64("fromBlock", fromBlock).Int64("toBlock", toBlock).
		Int64("blockRange", blockRange).Int("eventCount", len(events)).
		Dur("duration", time.Since(start)).Msg("geckoterminal events completed")

	return c.JSON(http.StatusOK, EventsResponse{Events: events})
}

// findPool returns the pool matching the given asset ID, or nil.
func findPool(pools []openapi.Pool, assetID string) *openapi.Pool {
	for _, pool := range pools {
		if pool.Asset == assetID {
			return &pool
		}
	}
	return nil
}

// parseAssetID extracts a name and symbol from a THORChain asset ID.
// Format: CHAIN.SYMBOL or CHAIN.SYMBOL-CONTRACT
func parseAssetID(assetID string) (name, symbol string) {
	name = assetID
	symbol = assetID
	if parts := strings.SplitN(assetID, ".", 2); len(parts) == 2 {
		symbol = parts[1]
		if dashIdx := strings.Index(symbol, "-"); dashIdx != -1 {
			symbol = symbol[:dashIdx]
		}
	}
	return name, symbol
}

// getMimirFee returns the swap fee in basis points from the Mimir config cache.
func getMimirFee() *int {
	mimirs := CachedThornodeThorchainMimir()
	if mimirs != nil {
		if fee, exists := mimirs["L1SLIPMINBPS"]; exists {
			feeBps := int(fee)
			return &feeBps
		}
	}
	defaultFee := 100
	log.Warn().Int("defaultFee", defaultFee).
		Msg("L1SLIPMINBPS not found in mimir cache, using default")
	return &defaultFee
}

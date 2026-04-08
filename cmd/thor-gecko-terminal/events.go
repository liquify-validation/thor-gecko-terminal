package main

import (
	"fmt"
	"math"

	"github.com/rs/zerolog/log"
)

// e8ToDecimal converts a 1e8 integer amount to a decimal string with 8 decimal places.
func e8ToDecimal(e8 int64) string {
	return fmt.Sprintf("%.8f", float64(e8)/1e8)
}

// makeReserves builds a Reserves struct from pool depth values.
// Reserves represent the end-of-block pool state from midgard.block_pool_depths.
// Per-event granularity is not available; this is the closest approximation.
func makeReserves(poolRuneE8, poolAssetE8 int64) Reserves {
	return Reserves{
		Asset0: e8ToDecimal(poolRuneE8),  // RUNE is always asset0
		Asset1: e8ToDecimal(poolAssetE8), // External asset is asset1
	}
}

// calculateReservePrice derives the price of asset0 (RUNE) in terms of asset1
// from pool reserves as a fallback when swap amounts can't produce a valid price.
func calculateReservePrice(poolRuneE8, poolAssetE8 int64) string {
	if poolRuneE8 > 0 && poolAssetE8 > 0 {
		price := float64(poolAssetE8) / float64(poolRuneE8)
		if price > 0 && !math.IsInf(price, 0) && !math.IsNaN(price) {
			return fmt.Sprintf("%.18f", price)
		}
	}
	return ""
}

// isValidPrice checks whether a calculated price is reasonable for serialization.
func isValidPrice(price float64) bool {
	return price > 0 && price < 1e15 && !math.IsInf(price, 0) && !math.IsNaN(price)
}

// transformSwapEvent converts a raw swap event into the GeckoTerminal API format.
// Returns (event, valid). Invalid events (no calculable price) should be skipped
// to avoid halting GeckoTerminal's indexer (spec: priceNative=0 is an error condition).
func transformSwapEvent(block Block, e *rawEvent, txnIndex, eventIndex int, poolRuneE8, poolAssetE8 int64) (SwapEvent, bool) {
	fromAmount := e8ToDecimal(e.FromE8)
	toAmount := e8ToDecimal(e.ToE8)

	event := SwapEvent{
		Block:      block,
		EventType:  "swap",
		TxnID:      e.TxnID,
		TxnIndex:   txnIndex,
		EventIndex: eventIndex,
		Maker:      e.Maker,
		PairID:     e.PairID,
		Reserves:   makeReserves(poolRuneE8, poolAssetE8),
	}

	// THORChain direction logic: RUNE is always asset0.
	switch {
	case e.FromAsset == "THOR.RUNE":
		// RUNE → Asset swap
		event.Asset0In = &fromAmount
		event.Asset1Out = &toAmount
		if e.FromE8 > 0 && e.ToE8 > 0 {
			if price := float64(e.ToE8) / float64(e.FromE8); isValidPrice(price) {
				event.PriceNative = fmt.Sprintf("%.18f", price)
			}
		}

	case e.ToAsset == "THOR.RUNE":
		// Asset → RUNE swap
		event.Asset1In = &fromAmount
		event.Asset0Out = &toAmount
		if e.ToE8 > 0 && e.FromE8 > 0 {
			if price := float64(e.FromE8) / float64(e.ToE8); isValidPrice(price) {
				event.PriceNative = fmt.Sprintf("%.18f", price)
			}
		}

	default:
		log.Error().Str("fromAsset", e.FromAsset).Str("toAsset", e.ToAsset).
			Str("txnID", e.TxnID).Msg("unexpected swap direction - neither from nor to RUNE")
		return event, false
	}

	// Fall back to reserve-derived price if swap amounts didn't produce one.
	if event.PriceNative == "" {
		if reservePrice := calculateReservePrice(poolRuneE8, poolAssetE8); reservePrice != "" {
			log.Warn().Str("txnID", e.TxnID).Int64("fromE8", e.FromE8).Int64("toE8", e.ToE8).
				Str("reservePrice", reservePrice).Msg("using reserve-based fallback price")
			event.PriceNative = reservePrice
		} else {
			log.Warn().Str("txnID", e.TxnID).Int64("fromE8", e.FromE8).Int64("toE8", e.ToE8).
				Msg("skipping swap event - no valid price from swap or reserves")
			return event, false
		}
	}

	return event, true
}

// transformLiquidityEvent converts a raw join/exit event into the GeckoTerminal API format.
func transformLiquidityEvent(block Block, e *rawEvent, txnIndex, eventIndex int, poolRuneE8, poolAssetE8 int64) JoinExitEvent {
	event := JoinExitEvent{
		Block:      block,
		EventType:  e.EventType,
		TxnID:      e.TxnID,
		TxnIndex:   txnIndex,
		EventIndex: eventIndex,
		Maker:      e.Maker,
		PairID:     e.PairID,
		Reserves:   makeReserves(poolRuneE8, poolAssetE8),
	}

	switch e.EventType {
	case "join":
		event.Amount0 = e8ToDecimal(e.RuneE8)
		event.Amount1 = e8ToDecimal(e.AssetE8)
	case "exit":
		event.Amount0 = e8ToDecimal(e.EmitRuneE8)
		event.Amount1 = e8ToDecimal(e.EmitAssetE8)
	}

	return event
}

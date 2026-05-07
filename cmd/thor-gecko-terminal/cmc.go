// CoinMarketCap (CMC) Integration API - Helpers and shared logic.

package main

import (
	"strconv"
	"strings"
)

// extractChainSymbol splits a THORChain asset ID into its chain and symbol parts.
// Examples:
//
//	"BTC.BTC"                                  -> ("BTC", "BTC")
//	"ETH.USDC-0X..."                           -> ("ETH", "USDC")
//	"BSC.BNB"                                  -> ("BSC", "BNB")
func extractChainSymbol(assetID string) (chain, symbol string) {
	parts := strings.SplitN(assetID, ".", 2)
	if len(parts) != 2 {
		return "", assetID
	}
	chain = parts[0]
	symbol = parts[1]
	if dashIdx := strings.Index(symbol, "-"); dashIdx != -1 {
		symbol = symbol[:dashIdx]
	}
	return chain, symbol
}

// extractContractAddress returns the contract address suffix of a token asset ID,
// or "" if the asset has no contract suffix (e.g. native L1 assets).
func extractContractAddress(assetID string) string {
	if dashIdx := strings.Index(assetID, "-"); dashIdx != -1 {
		return assetID[dashIdx+1:]
	}
	return ""
}

// buildPairIDMap builds a map of THORChain asset ID -> CMC trading pair identifier.
// When multiple pools share the same symbol (e.g. USDC across chains), a chain
// suffix is appended to disambiguate (e.g. "RUNE_USDC-ETH").
func buildPairIDMap(pools []MidgardPool) map[string]string {
	symbolCounts := make(map[string]int)
	for _, p := range pools {
		_, sym := extractChainSymbol(p.Asset)
		symbolCounts[sym]++
	}

	pairIDs := make(map[string]string, len(pools))
	for _, p := range pools {
		chain, sym := extractChainSymbol(p.Asset)
		if symbolCounts[sym] > 1 {
			pairIDs[p.Asset] = "RUNE_" + sym + "-" + chain
		} else {
			pairIDs[p.Asset] = "RUNE_" + sym
		}
	}
	return pairIDs
}

// findAssetByPairID reverses a CMC trading pair identifier back to a THORChain
// asset ID by searching the pair ID map. Returns "" if no match.
func findAssetByPairID(pairID string, pairIDMap map[string]string) string {
	for asset, pid := range pairIDMap {
		if pid == pairID {
			return asset
		}
	}
	return ""
}

// parseFloatOrZero parses a string as float64, returning 0 on error or empty string.
func parseFloatOrZero(s string) float64 {
	if s == "" {
		return 0
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return v
}

// parseIntOrZero parses a string as int64, returning 0 on error or empty string.
func parseIntOrZero(s string) int64 {
	if s == "" {
		return 0
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0
	}
	return v
}

// e8ToFloat converts a 1e8 integer-string amount to a float64 decimal.
func e8ToFloat(s string) float64 {
	return parseFloatOrZero(s) / 1e8
}

// contractAddressURL returns a block-explorer URL for a token's contract address,
// based on its chain. Returns "" if no explorer is known for the chain.
func contractAddressURL(chain, contract string) string {
	if contract == "" {
		return ""
	}
	switch chain {
	case "ETH":
		return "https://etherscan.io/token/" + contract
	case "BSC":
		return "https://bscscan.com/token/" + contract
	case "AVAX":
		return "https://snowtrace.io/token/" + contract
	case "BASE":
		return "https://basescan.org/token/" + contract
	case "TRON":
		return "https://tronscan.org/#/token20/" + contract
	}
	return ""
}

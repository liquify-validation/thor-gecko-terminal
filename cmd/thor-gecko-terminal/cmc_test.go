package main

import (
	"encoding/json"
	"net/http"
	"testing"
)

func setTestMidgardPools(pools []MidgardPool) {
	midgardPoolsMu.Lock()
	defer midgardPoolsMu.Unlock()
	midgardPools = pools
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func TestExtractChainSymbol(t *testing.T) {
	tests := []struct {
		input          string
		expectedChain  string
		expectedSymbol string
	}{
		{"BTC.BTC", "BTC", "BTC"},
		{"ETH.USDC-0XA0B86991", "ETH", "USDC"},
		{"THOR.RUNE", "THOR", "RUNE"},
		{"BSC.BNB", "BSC", "BNB"},
		{"NODOT", "", "NODOT"},
	}
	for _, tt := range tests {
		chain, sym := extractChainSymbol(tt.input)
		if chain != tt.expectedChain || sym != tt.expectedSymbol {
			t.Errorf("extractChainSymbol(%q) = (%q, %q), want (%q, %q)",
				tt.input, chain, sym, tt.expectedChain, tt.expectedSymbol)
		}
	}
}

func TestExtractContractAddress(t *testing.T) {
	if got := extractContractAddress("BTC.BTC"); got != "" {
		t.Errorf("expected empty contract for native asset, got %q", got)
	}
	if got := extractContractAddress("ETH.USDC-0XA0B86991"); got != "0XA0B86991" {
		t.Errorf("expected 0XA0B86991, got %q", got)
	}
}

func TestBuildPairIDMap_NoCollisions(t *testing.T) {
	pools := []MidgardPool{
		{Asset: "BTC.BTC"},
		{Asset: "ETH.ETH"},
		{Asset: "THOR.RUJI"},
	}
	pairIDs := buildPairIDMap(pools)
	if pairIDs["BTC.BTC"] != "RUNE_BTC" {
		t.Errorf("expected RUNE_BTC, got %q", pairIDs["BTC.BTC"])
	}
	if pairIDs["ETH.ETH"] != "RUNE_ETH" {
		t.Errorf("expected RUNE_ETH, got %q", pairIDs["ETH.ETH"])
	}
	if pairIDs["THOR.RUJI"] != "RUNE_RUJI" {
		t.Errorf("expected RUNE_RUJI, got %q", pairIDs["THOR.RUJI"])
	}
}

func TestBuildPairIDMap_CollisionsDisambiguated(t *testing.T) {
	pools := []MidgardPool{
		{Asset: "ETH.USDC-0XA0B86991"},
		{Asset: "BSC.USDC-0X8AC76A51"},
		{Asset: "BTC.BTC"}, // unique
	}
	pairIDs := buildPairIDMap(pools)
	if pairIDs["ETH.USDC-0XA0B86991"] != "RUNE_USDC-ETH" {
		t.Errorf("expected RUNE_USDC-ETH, got %q", pairIDs["ETH.USDC-0XA0B86991"])
	}
	if pairIDs["BSC.USDC-0X8AC76A51"] != "RUNE_USDC-BSC" {
		t.Errorf("expected RUNE_USDC-BSC, got %q", pairIDs["BSC.USDC-0X8AC76A51"])
	}
	if pairIDs["BTC.BTC"] != "RUNE_BTC" {
		t.Errorf("expected RUNE_BTC (no collision), got %q", pairIDs["BTC.BTC"])
	}
}

func TestFindAssetByPairID(t *testing.T) {
	pairIDs := map[string]string{
		"BTC.BTC": "RUNE_BTC",
		"ETH.ETH": "RUNE_ETH",
	}
	if got := findAssetByPairID("RUNE_BTC", pairIDs); got != "BTC.BTC" {
		t.Errorf("expected BTC.BTC, got %q", got)
	}
	if got := findAssetByPairID("RUNE_UNKNOWN", pairIDs); got != "" {
		t.Errorf("expected empty for unknown pair, got %q", got)
	}
}

func TestParseFloatOrZero(t *testing.T) {
	if got := parseFloatOrZero(""); got != 0 {
		t.Errorf("expected 0 for empty, got %f", got)
	}
	if got := parseFloatOrZero("garbage"); got != 0 {
		t.Errorf("expected 0 for invalid, got %f", got)
	}
	if got := parseFloatOrZero("3.14"); got != 3.14 {
		t.Errorf("expected 3.14, got %f", got)
	}
}

func TestE8ToFloat(t *testing.T) {
	if got := e8ToFloat("100000000"); got != 1.0 {
		t.Errorf("expected 1.0, got %f", got)
	}
	if got := e8ToFloat("50000000"); got != 0.5 {
		t.Errorf("expected 0.5, got %f", got)
	}
}

func TestContractAddressURL(t *testing.T) {
	if got := contractAddressURL("ETH", "0xABC"); got != "https://etherscan.io/token/0xABC" {
		t.Errorf("ETH explorer url wrong: %q", got)
	}
	if got := contractAddressURL("BSC", "0xDEF"); got != "https://bscscan.com/token/0xDEF" {
		t.Errorf("BSC explorer url wrong: %q", got)
	}
	if got := contractAddressURL("ETH", ""); got != "" {
		t.Errorf("expected empty for no contract, got %q", got)
	}
	if got := contractAddressURL("UNKNOWN", "0x123"); got != "" {
		t.Errorf("expected empty for unknown chain, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// /summary
// ---------------------------------------------------------------------------

func TestCMCSummary_NoData(t *testing.T) {
	setTestMidgardPools(nil)
	c, rec := newTestContext(http.MethodGet, "/thorchain/cmc/summary")
	if err := CMCSummary(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}

func TestCMCSummary_ReturnsAllAvailablePools(t *testing.T) {
	setTestMidgardPools([]MidgardPool{
		{
			Asset:      "BTC.BTC",
			Status:     "available",
			AssetPrice: "147225",          // 1 BTC = 147225 RUNE
			Volume24h:  "100000000000000", // 1,000,000 RUNE
		},
		{
			Asset:      "ETH.ETH",
			Status:     "available",
			AssetPrice: "10000", // 1 ETH = 10000 RUNE
			Volume24h:  "50000000000000",
		},
		{
			Asset:      "DEAD.POOL",
			Status:     "staged",
			AssetPrice: "100",
			Volume24h:  "0",
		},
	})

	c, rec := newTestContext(http.MethodGet, "/thorchain/cmc/summary")
	if err := CMCSummary(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp []CMCSummaryEntry
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp) != 2 {
		t.Fatalf("expected 2 available pools, got %d", len(resp))
	}

	// Find BTC entry
	var btc *CMCSummaryEntry
	for i := range resp {
		if resp[i].QuoteCurrency == "BTC" {
			btc = &resp[i]
		}
	}
	if btc == nil {
		t.Fatal("expected BTC entry")
	}
	if btc.TradingPairs != "RUNE_BTC" {
		t.Errorf("expected RUNE_BTC, got %s", btc.TradingPairs)
	}
	if btc.BaseCurrency != "RUNE" {
		t.Errorf("expected base RUNE, got %s", btc.BaseCurrency)
	}
	// 1/147225 BTC per RUNE
	expectedPrice := 1.0 / 147225.0
	if absDiff(btc.LastPrice, expectedPrice) > 1e-12 {
		t.Errorf("expected last_price ~%g, got %g", expectedPrice, btc.LastPrice)
	}
	if btc.LowestAsk != btc.LastPrice {
		t.Error("expected lowest_ask == last_price for AMM")
	}
}

// ---------------------------------------------------------------------------
// /assets
// ---------------------------------------------------------------------------

func TestCMCAssets_NoData(t *testing.T) {
	setTestMidgardPools(nil)
	c, rec := newTestContext(http.MethodGet, "/thorchain/cmc/assets")
	if err := CMCAssets(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}

func TestCMCAssets_IncludesRUNEAndPoolAssets(t *testing.T) {
	setTestMidgardPools([]MidgardPool{
		{Asset: "BTC.BTC", Status: "available"},
		{Asset: "ETH.USDC-0XA0B86991", Status: "available"},
	})

	c, rec := newTestContext(http.MethodGet, "/thorchain/cmc/assets")
	if err := CMCAssets(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var assets map[string]CMCAsset
	if err := json.Unmarshal(rec.Body.Bytes(), &assets); err != nil {
		t.Fatal(err)
	}

	if _, ok := assets["RUNE"]; !ok {
		t.Fatal("expected RUNE in assets")
	}
	if _, ok := assets["BTC"]; !ok {
		t.Fatal("expected BTC in assets")
	}
	usdc, ok := assets["USDC"]
	if !ok {
		t.Fatal("expected USDC in assets")
	}
	if usdc.ContractAddress != "0XA0B86991" {
		t.Errorf("expected contract 0XA0B86991, got %q", usdc.ContractAddress)
	}
	if usdc.ContractAddressURL == "" {
		t.Error("expected contract URL to be set for ETH token")
	}
}

// ---------------------------------------------------------------------------
// /ticker
// ---------------------------------------------------------------------------

func TestCMCTicker_NoData(t *testing.T) {
	setTestMidgardPools(nil)
	c, rec := newTestContext(http.MethodGet, "/thorchain/cmc/ticker")
	if err := CMCTicker(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}

func TestCMCTicker_KeyedByPairID(t *testing.T) {
	setTestMidgardPools([]MidgardPool{
		{Asset: "BTC.BTC", Status: "available", AssetPrice: "147225", Volume24h: "100000000"},
		{Asset: "ETH.ETH", Status: "available", AssetPrice: "10000", Volume24h: "200000000"},
	})

	c, rec := newTestContext(http.MethodGet, "/thorchain/cmc/ticker")
	if err := CMCTicker(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var tickers map[string]CMCTickerEntry
	if err := json.Unmarshal(rec.Body.Bytes(), &tickers); err != nil {
		t.Fatal(err)
	}
	if _, ok := tickers["RUNE_BTC"]; !ok {
		t.Fatal("expected RUNE_BTC in tickers")
	}
	if _, ok := tickers["RUNE_ETH"]; !ok {
		t.Fatal("expected RUNE_ETH in tickers")
	}
}

// ---------------------------------------------------------------------------
// /trades
// ---------------------------------------------------------------------------

func TestCMCTrades_MissingMarketPair(t *testing.T) {
	c, rec := newTestContext(http.MethodGet, "/thorchain/cmc/trades")
	if err := CMCTrades(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestCMCTrades_NoDB(t *testing.T) {
	midgardDB = nil
	c, rec := newTestContext(http.MethodGet, "/thorchain/cmc/trades?market_pair=RUNE_BTC")
	c.QueryParams().Set("market_pair", "RUNE_BTC")

	if err := CMCTrades(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}

func TestBuildTradeEntry_RuneToAsset(t *testing.T) {
	trade := buildTradeEntry("TX1", "THOR.RUNE", 1000000000, "BTC.BTC", 5000, 1700000000_000_000_000)
	if trade == nil {
		t.Fatal("expected trade")
	}
	if trade.Type != "sell" {
		t.Errorf("expected sell, got %s", trade.Type)
	}
	if trade.BaseVolume != 10.0 {
		t.Errorf("expected baseVolume 10, got %f", trade.BaseVolume)
	}
	if trade.QuoteVolume != 0.00005 {
		t.Errorf("expected quoteVolume 0.00005, got %f", trade.QuoteVolume)
	}
	if trade.Timestamp != 1700000000_000 {
		t.Errorf("expected ms timestamp, got %d", trade.Timestamp)
	}
}

func TestBuildTradeEntry_AssetToRune(t *testing.T) {
	trade := buildTradeEntry("TX2", "BTC.BTC", 5000, "THOR.RUNE", 1000000000, 1700000000_000_000_000)
	if trade == nil {
		t.Fatal("expected trade")
	}
	if trade.Type != "buy" {
		t.Errorf("expected buy, got %s", trade.Type)
	}
	if trade.BaseVolume != 10.0 {
		t.Errorf("expected baseVolume 10 RUNE, got %f", trade.BaseVolume)
	}
}

// ---------------------------------------------------------------------------
// /swaps
// ---------------------------------------------------------------------------

func TestCMCSwaps_NoDB(t *testing.T) {
	midgardDB = nil
	c, rec := newTestContext(http.MethodGet, "/thorchain/cmc/swaps")
	if err := CMCSwaps(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}

// ---------------------------------------------------------------------------

func absDiff(a, b float64) float64 {
	if a > b {
		return a - b
	}
	return b - a
}

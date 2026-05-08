package main

import (
	"encoding/json"
	"math"
	"net/http"
	"testing"
)

// ---------------------------------------------------------------------------
// synthesizeOrderbook
// ---------------------------------------------------------------------------

func TestSynthesizeOrderbook_BalancedPool(t *testing.T) {
	// 1000 RUNE × 10 BTC, k=10000, spot = 0.01 BTC/RUNE
	bids, asks := synthesizeOrderbook(1000, 10, 5, 0.01)

	if len(asks) != 5 || len(bids) != 5 {
		t.Fatalf("expected 5 levels each side, got bids=%d asks=%d", len(bids), len(asks))
	}

	// Asks must be ascending in price.
	for i := 1; i < len(asks); i++ {
		if asks[i][0] <= asks[i-1][0] {
			t.Errorf("asks not ascending: asks[%d]=%v asks[%d]=%v", i-1, asks[i-1], i, asks[i])
		}
	}

	// Bids must be descending in price (best bid first = closest to spot).
	for i := 1; i < len(bids); i++ {
		if bids[i][0] >= bids[i-1][0] {
			t.Errorf("bids not descending: bids[%d]=%v bids[%d]=%v", i-1, bids[i-1], i, bids[i])
		}
	}

	// All quantities positive.
	for i, level := range asks {
		if level[1] <= 0 {
			t.Errorf("asks[%d] non-positive quantity: %v", i, level)
		}
	}
	for i, level := range bids {
		if level[1] <= 0 {
			t.Errorf("bids[%d] non-positive quantity: %v", i, level)
		}
	}

	// Best ask price > spot, best bid price < spot.
	spot := 10.0 / 1000.0
	if asks[0][0] <= spot {
		t.Errorf("best ask %v should be > spot %v", asks[0][0], spot)
	}
	if bids[0][0] >= spot {
		t.Errorf("best bid %v should be < spot %v", bids[0][0], spot)
	}
}

func TestSynthesizeOrderbook_PreservesConstantProduct(t *testing.T) {
	// Sum of all ask quantities should equal cumulative RUNE removed
	// from the pool, which can be derived from the final price level.
	x := 1000.0
	y := 10.0
	k := x * y

	bids, asks := synthesizeOrderbook(x, y, 10, 0.01)

	var totalAskRune float64
	for _, level := range asks {
		totalAskRune += level[1]
	}
	// Final ask price determines final x via x' = sqrt(k/P').
	finalAskPrice := asks[len(asks)-1][0]
	expectedX := math.Sqrt(k / finalAskPrice)
	expectedRemoved := x - expectedX
	if math.Abs(totalAskRune-expectedRemoved) > 1e-6 {
		t.Errorf("ask sum %v != expected %v", totalAskRune, expectedRemoved)
	}

	var totalBidRune float64
	for _, level := range bids {
		totalBidRune += level[1]
	}
	finalBidPrice := bids[len(bids)-1][0]
	expectedXBids := math.Sqrt(k / finalBidPrice)
	expectedAdded := expectedXBids - x
	if math.Abs(totalBidRune-expectedAdded) > 1e-6 {
		t.Errorf("bid sum %v != expected %v", totalBidRune, expectedAdded)
	}
}

func TestSynthesizeOrderbook_InvalidInputs(t *testing.T) {
	if bids, asks := synthesizeOrderbook(0, 10, 5, 0.01); bids != nil || asks != nil {
		t.Error("expected nil for zero rune depth")
	}
	if bids, asks := synthesizeOrderbook(10, 0, 5, 0.01); bids != nil || asks != nil {
		t.Error("expected nil for zero asset depth")
	}
	if bids, asks := synthesizeOrderbook(10, 10, 0, 0.01); bids != nil || asks != nil {
		t.Error("expected nil for zero levels")
	}
}

// ---------------------------------------------------------------------------
// resolveLevels
// ---------------------------------------------------------------------------

func TestResolveLevels(t *testing.T) {
	tests := []struct {
		level, depth string
		want         int
	}{
		{"1", "", 1},                    // top of book
		{"3", "", maxOrderbookLevels},   // deep
		{"", "100", 50},                 // 50 per side
		{"", "10", 5},                   // 5 per side
		{"", "0", defaultOrderbookLevels},
		{"", "", defaultOrderbookLevels},
		{"", "9999", maxOrderbookLevels}, // capped
	}
	for _, tt := range tests {
		got := resolveLevels(tt.level, tt.depth)
		if got != tt.want {
			t.Errorf("resolveLevels(%q, %q) = %d, want %d", tt.level, tt.depth, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// /orderbook handler
// ---------------------------------------------------------------------------

func TestCMCOrderbook_MissingMarketPair(t *testing.T) {
	c, rec := newTestContext(http.MethodGet, "/thorchain/cmc/orderbook")
	if err := CMCOrderbookHandler(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestCMCOrderbook_NoData(t *testing.T) {
	setTestMidgardPools(nil)
	c, rec := newTestContext(http.MethodGet, "/thorchain/cmc/orderbook?market_pair=RUNE_BTC")
	c.QueryParams().Set("market_pair", "RUNE_BTC")
	if err := CMCOrderbookHandler(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}

func TestCMCOrderbook_UnknownPair(t *testing.T) {
	setTestMidgardPools([]MidgardPool{{Asset: "BTC.BTC", Status: "available"}})
	c, rec := newTestContext(http.MethodGet, "/thorchain/cmc/orderbook?market_pair=RUNE_FAKE")
	c.QueryParams().Set("market_pair", "RUNE_FAKE")
	if err := CMCOrderbookHandler(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestCMCOrderbook_ReturnsLevels(t *testing.T) {
	setTestMidgardPools([]MidgardPool{
		{
			Asset:      "BTC.BTC",
			Status:     "available",
			RuneDepth:  "100000000000", // 1000 RUNE
			AssetDepth: "1000000000",   // 10 BTC
		},
	})

	c, rec := newTestContext(http.MethodGet, "/thorchain/cmc/orderbook?market_pair=RUNE_BTC&depth=20")
	c.QueryParams().Set("market_pair", "RUNE_BTC")
	c.QueryParams().Set("depth", "20")

	if err := CMCOrderbookHandler(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp CMCOrderbook
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}

	if len(resp.Bids) != 10 || len(resp.Asks) != 10 {
		t.Errorf("expected 10 bids and 10 asks (depth=20), got bids=%d asks=%d",
			len(resp.Bids), len(resp.Asks))
	}
	if resp.Timestamp == 0 {
		t.Error("expected non-zero timestamp")
	}
}

func TestCMCOrderbook_LevelOneReturnsTopOfBook(t *testing.T) {
	setTestMidgardPools([]MidgardPool{
		{
			Asset:      "BTC.BTC",
			Status:     "available",
			RuneDepth:  "100000000000",
			AssetDepth: "1000000000",
		},
	})

	c, rec := newTestContext(http.MethodGet, "/thorchain/cmc/orderbook?market_pair=RUNE_BTC&level=1")
	c.QueryParams().Set("market_pair", "RUNE_BTC")
	c.QueryParams().Set("level", "1")

	if err := CMCOrderbookHandler(c); err != nil {
		t.Fatal(err)
	}

	var resp CMCOrderbook
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}

	if len(resp.Bids) != 1 || len(resp.Asks) != 1 {
		t.Errorf("expected 1 each side for level=1, got bids=%d asks=%d",
			len(resp.Bids), len(resp.Asks))
	}
}

func TestCMCOrderbook_NoLiquidity(t *testing.T) {
	setTestMidgardPools([]MidgardPool{
		{Asset: "BTC.BTC", Status: "available", RuneDepth: "0", AssetDepth: "0"},
	})

	c, rec := newTestContext(http.MethodGet, "/thorchain/cmc/orderbook?market_pair=RUNE_BTC")
	c.QueryParams().Set("market_pair", "RUNE_BTC")

	if err := CMCOrderbookHandler(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 for empty pool, got %d", rec.Code)
	}
}

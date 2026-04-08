package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	echo "github.com/labstack/echo/v4"
	openapi "gitlab.com/thorchain/thornode/v3/openapi/gen"
)

// setTestPools sets the cached pools for testing.
func setTestPools(pools []openapi.Pool) {
	thornodeThorchainPoolsMu.Lock()
	defer thornodeThorchainPoolsMu.Unlock()
	thornodeThorchainPools = pools
}

// setTestMimir sets the cached mimir config for testing.
func setTestMimir(m map[string]int64) {
	thornodeThorchainMimirMu.Lock()
	defer thornodeThorchainMimirMu.Unlock()
	thornodeThorchainMimir = m
}

func newTestContext(method, path string) (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(method, path, nil)
	rec := httptest.NewRecorder()
	return e.NewContext(req, rec), rec
}

// ---------------------------------------------------------------------------
// /latest-block
// ---------------------------------------------------------------------------

func TestLatestBlock_NoDB(t *testing.T) {
	midgardDB = nil
	c, rec := newTestContext(http.MethodGet, "/thorchain/geckoterminal/latest-block")

	if err := GeckoterminalLatestBlock(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// /asset
// ---------------------------------------------------------------------------

func TestAsset_MissingID(t *testing.T) {
	c, rec := newTestContext(http.MethodGet, "/thorchain/geckoterminal/asset")

	if err := GeckoterminalAsset(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestAsset_RUNE(t *testing.T) {
	setTestPools([]openapi.Pool{{Asset: "BTC.BTC", Status: "Available", BalanceAsset: "1000000000"}})

	c, rec := newTestContext(http.MethodGet, "/thorchain/geckoterminal/asset?id=THOR.RUNE")
	c.QueryParams().Set("id", "THOR.RUNE")

	if err := GeckoterminalAsset(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp AssetResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Asset.ID != "THOR.RUNE" {
		t.Fatalf("expected THOR.RUNE, got %s", resp.Asset.ID)
	}
	if resp.Asset.Symbol != "RUNE" {
		t.Fatalf("expected symbol RUNE, got %s", resp.Asset.Symbol)
	}
	if resp.Asset.Decimals != 8 {
		t.Fatalf("expected 8 decimals, got %d", resp.Asset.Decimals)
	}
	if resp.Asset.CoinGeckoID == nil || *resp.Asset.CoinGeckoID != "thorchain" {
		t.Fatal("expected coinGeckoId thorchain")
	}
}

func TestAsset_KnownPool(t *testing.T) {
	setTestPools([]openapi.Pool{
		{Asset: "BTC.BTC", Status: "Available", BalanceAsset: "500000000000"},
	})

	c, rec := newTestContext(http.MethodGet, "/thorchain/geckoterminal/asset?id=BTC.BTC")
	c.QueryParams().Set("id", "BTC.BTC")

	if err := GeckoterminalAsset(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp AssetResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Asset.ID != "BTC.BTC" {
		t.Fatalf("expected BTC.BTC, got %s", resp.Asset.ID)
	}
	if resp.Asset.Name != "Bitcoin" {
		t.Fatalf("expected name Bitcoin, got %s", resp.Asset.Name)
	}
	if resp.Asset.Symbol != "BTC" {
		t.Fatalf("expected symbol BTC, got %s", resp.Asset.Symbol)
	}
	if resp.Asset.CoinGeckoID == nil || *resp.Asset.CoinGeckoID != "bitcoin" {
		t.Fatal("expected coinGeckoId bitcoin")
	}
	if resp.Asset.TotalSupply == nil {
		t.Fatal("expected totalSupply to be set")
	}
	if resp.Asset.CirculatingSupply == nil {
		t.Fatal("expected circulatingSupply to be set")
	}
}

func TestAsset_TokenWithContract(t *testing.T) {
	setTestPools([]openapi.Pool{
		{Asset: "ETH.USDC-0XA0B86991C6218B36C1D19D4A2E9EB0CE3606EB48", Status: "Available", BalanceAsset: "100000000"},
	})

	c, rec := newTestContext(http.MethodGet, "/thorchain/geckoterminal/asset?id=ETH.USDC-0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48")
	c.QueryParams().Set("id", "ETH.USDC-0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48")

	if err := GeckoterminalAsset(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp AssetResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Asset.Symbol != "USDC" {
		t.Fatalf("expected symbol USDC, got %s", resp.Asset.Symbol)
	}
	if resp.Asset.CoinGeckoID == nil || *resp.Asset.CoinGeckoID != "usd-coin" {
		t.Fatal("expected coinGeckoId usd-coin")
	}
}

func TestAsset_UnknownPool(t *testing.T) {
	setTestPools([]openapi.Pool{
		{Asset: "BTC.BTC", Status: "Available", BalanceAsset: "100000000"},
	})

	c, rec := newTestContext(http.MethodGet, "/thorchain/geckoterminal/asset?id=FAKE.TOKEN")
	c.QueryParams().Set("id", "FAKE.TOKEN")

	if err := GeckoterminalAsset(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for unknown asset, got %d", rec.Code)
	}

	var resp AssetResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Asset.Symbol != "TOKEN" {
		t.Fatalf("expected symbol TOKEN, got %s", resp.Asset.Symbol)
	}
}

func TestAsset_EmptyCache(t *testing.T) {
	setTestPools(nil)

	c, rec := newTestContext(http.MethodGet, "/thorchain/geckoterminal/asset?id=BTC.BTC")
	c.QueryParams().Set("id", "BTC.BTC")

	if err := GeckoterminalAsset(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 when cache empty, got %d", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// /pair
// ---------------------------------------------------------------------------

func TestPair_MissingID(t *testing.T) {
	c, rec := newTestContext(http.MethodGet, "/thorchain/geckoterminal/pair")

	if err := GeckoterminalPair(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestPair_KnownPool(t *testing.T) {
	setTestPools([]openapi.Pool{
		{Asset: "BTC.BTC", Status: "Available"},
	})
	setTestMimir(map[string]int64{"L1SLIPMINBPS": 50})

	c, rec := newTestContext(http.MethodGet, "/thorchain/geckoterminal/pair?id=BTC.BTC")
	c.QueryParams().Set("id", "BTC.BTC")

	if err := GeckoterminalPair(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp PairResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Pair.DexKey != "thorchain" {
		t.Fatalf("expected dexKey thorchain, got %s", resp.Pair.DexKey)
	}
	if resp.Pair.Asset0ID != "THOR.RUNE" {
		t.Fatalf("expected asset0Id THOR.RUNE, got %s", resp.Pair.Asset0ID)
	}
	if resp.Pair.Asset1ID != "BTC.BTC" {
		t.Fatalf("expected asset1Id BTC.BTC, got %s", resp.Pair.Asset1ID)
	}
	if resp.Pair.FeeBps == nil || *resp.Pair.FeeBps != 50 {
		t.Fatalf("expected feeBps 50, got %v", resp.Pair.FeeBps)
	}
}

func TestPair_UnknownPool(t *testing.T) {
	setTestPools([]openapi.Pool{
		{Asset: "BTC.BTC", Status: "Available"},
	})

	c, rec := newTestContext(http.MethodGet, "/thorchain/geckoterminal/pair?id=FAKE.TOKEN")
	c.QueryParams().Set("id", "FAKE.TOKEN")

	if err := GeckoterminalPair(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for unknown pair, got %d", rec.Code)
	}

	var resp PairResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Pair.Asset0ID != "THOR.RUNE" {
		t.Fatalf("expected asset0Id THOR.RUNE, got %s", resp.Pair.Asset0ID)
	}
	if resp.Pair.FeeBps != nil {
		t.Fatal("expected nil feeBps for unknown pair")
	}
}

func TestPair_DefaultFee(t *testing.T) {
	setTestPools([]openapi.Pool{
		{Asset: "ETH.ETH", Status: "Available"},
	})
	setTestMimir(map[string]int64{}) // no L1SLIPMINBPS

	c, rec := newTestContext(http.MethodGet, "/thorchain/geckoterminal/pair?id=ETH.ETH")
	c.QueryParams().Set("id", "ETH.ETH")

	if err := GeckoterminalPair(c); err != nil {
		t.Fatal(err)
	}

	var resp PairResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Pair.FeeBps == nil || *resp.Pair.FeeBps != 100 {
		t.Fatalf("expected default feeBps 100, got %v", resp.Pair.FeeBps)
	}
}

// ---------------------------------------------------------------------------
// /events
// ---------------------------------------------------------------------------

func TestEvents_MissingParams(t *testing.T) {
	c, rec := newTestContext(http.MethodGet, "/thorchain/geckoterminal/events")

	if err := GeckoterminalEvents(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestEvents_InvalidFromBlock(t *testing.T) {
	c, rec := newTestContext(http.MethodGet, "/thorchain/geckoterminal/events?fromBlock=abc&toBlock=100")
	c.QueryParams().Set("fromBlock", "abc")
	c.QueryParams().Set("toBlock", "100")

	if err := GeckoterminalEvents(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestEvents_InvalidToBlock(t *testing.T) {
	c, rec := newTestContext(http.MethodGet, "/thorchain/geckoterminal/events?fromBlock=100&toBlock=abc")
	c.QueryParams().Set("fromBlock", "100")
	c.QueryParams().Set("toBlock", "abc")

	if err := GeckoterminalEvents(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestEvents_FromGreaterThanTo(t *testing.T) {
	c, rec := newTestContext(http.MethodGet, "/thorchain/geckoterminal/events?fromBlock=200&toBlock=100")
	c.QueryParams().Set("fromBlock", "200")
	c.QueryParams().Set("toBlock", "100")

	if err := GeckoterminalEvents(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestEvents_RangeTooLarge(t *testing.T) {
	c, rec := newTestContext(http.MethodGet, "/thorchain/geckoterminal/events?fromBlock=1&toBlock=500")
	c.QueryParams().Set("fromBlock", "1")
	c.QueryParams().Set("toBlock", "500")

	if err := GeckoterminalEvents(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestEvents_NoDB(t *testing.T) {
	midgardDB = nil

	c, rec := newTestContext(http.MethodGet, "/thorchain/geckoterminal/events?fromBlock=1&toBlock=10")
	c.QueryParams().Set("fromBlock", "1")
	c.QueryParams().Set("toBlock", "10")

	if err := GeckoterminalEvents(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}

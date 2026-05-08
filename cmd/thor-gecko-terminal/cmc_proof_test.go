package main

import (
	"encoding/json"
	"net/http"
	"testing"

	openapi "gitlab.com/thorchain/thornode/v3/openapi/gen"
)

func setTestVaults(vaults []AsgardVault) {
	thornodeThorchainVaultsAsgardMu.Lock()
	defer thornodeThorchainVaultsAsgardMu.Unlock()
	thornodeThorchainVaultsAsgard = vaults
}

func setTestThornodePools(pools []openapi.Pool) {
	thornodeThorchainPoolsMu.Lock()
	defer thornodeThorchainPoolsMu.Unlock()
	thornodeThorchainPools = pools
}

// ---------------------------------------------------------------------------
// /proof-of-reserves
// ---------------------------------------------------------------------------

func TestProofOfReserves_NoData(t *testing.T) {
	setTestVaults(nil)
	c, rec := newTestContext(http.MethodGet, "/thorchain/cmc/proof-of-reserves")
	if err := CMCProofOfReservesHandler(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}

func TestProofOfReserves_AggregatesAcrossVaults(t *testing.T) {
	setTestVaults([]AsgardVault{
		{
			BlockHeight: 100,
			Status:      "ActiveVault",
			Addresses: []AsgardVaultAddr{
				{Chain: "BTC", Address: "bc1qaaa"},
				{Chain: "ETH", Address: "0xaaa"},
			},
			Coins: []AsgardCoin{
				{Asset: "BTC.BTC", Amount: "100000000"},  // 1 BTC
				{Asset: "ETH.ETH", Amount: "5000000000"}, // 50 ETH
			},
		},
		{
			BlockHeight: 101,
			Status:      "ActiveVault",
			Addresses: []AsgardVaultAddr{
				{Chain: "BTC", Address: "bc1qbbb"},
				{Chain: "ETH", Address: "0xbbb"},
			},
			Coins: []AsgardCoin{
				{Asset: "BTC.BTC", Amount: "200000000"},  // 2 BTC
				{Asset: "ETH.ETH", Amount: "1000000000"}, // 10 ETH
			},
		},
		{
			BlockHeight: 99,
			Status:      "RetiringVault",
			Addresses:   []AsgardVaultAddr{{Chain: "BTC", Address: "bc1qccc"}},
			Coins:       []AsgardCoin{{Asset: "BTC.BTC", Amount: "999000000"}}, // excluded
		},
	})

	c, rec := newTestContext(http.MethodGet, "/thorchain/cmc/proof-of-reserves")
	if err := CMCProofOfReservesHandler(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp CMCProofOfReserves
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}

	if resp.BlockHeight != 101 {
		t.Errorf("expected block_height 101 (latest active vault), got %d", resp.BlockHeight)
	}
	if resp.Network != "thorchain" {
		t.Errorf("expected network thorchain, got %s", resp.Network)
	}

	var btc *CMCReserve
	for i := range resp.Reserves {
		if resp.Reserves[i].Symbol == "BTC" {
			btc = &resp.Reserves[i]
		}
	}
	if btc == nil {
		t.Fatal("expected BTC reserve")
	}
	if btc.TotalReserve != "3.00000000" {
		t.Errorf("expected BTC total 3.00000000, got %s", btc.TotalReserve)
	}
	if len(btc.VaultAddresses) != 2 {
		t.Fatalf("expected 2 BTC vaults (retiring excluded), got %d", len(btc.VaultAddresses))
	}
	for _, va := range btc.VaultAddresses {
		if va.ExplorerURL == "" {
			t.Errorf("expected explorer URL for BTC vault %s", va.Address)
		}
	}
}

func TestProofOfReserves_SkipsSyntheticAssets(t *testing.T) {
	setTestVaults([]AsgardVault{
		{
			BlockHeight: 100,
			Status:      "ActiveVault",
			Coins: []AsgardCoin{
				{Asset: "BTC.BTC", Amount: "100000000"}, // L1
				{Asset: "BTC/BTC", Amount: "50000000"},  // synth — should be excluded
				{Asset: "BTC~BTC", Amount: "25000000"},  // trade — should be excluded
			},
		},
	})

	c, rec := newTestContext(http.MethodGet, "/thorchain/cmc/proof-of-reserves")
	if err := CMCProofOfReservesHandler(c); err != nil {
		t.Fatal(err)
	}

	var resp CMCProofOfReserves
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}

	if len(resp.Reserves) != 1 {
		t.Fatalf("expected 1 reserve (derivatives excluded), got %d", len(resp.Reserves))
	}
	if resp.Reserves[0].TotalReserve != "1.00000000" {
		t.Errorf("expected only L1 BTC counted, got total %s", resp.Reserves[0].TotalReserve)
	}
}

func TestProofOfReserves_VerificationURLUsesConfiguredAPI(t *testing.T) {
	thornodeAPIBase = "https://gateway.liquify.com/chain/thorchain_api"
	setTestVaults([]AsgardVault{
		{
			BlockHeight: 100,
			Status:      "ActiveVault",
			Coins:       []AsgardCoin{{Asset: "BTC.BTC", Amount: "100000000"}},
		},
	})

	c, rec := newTestContext(http.MethodGet, "/thorchain/cmc/proof-of-reserves")
	if err := CMCProofOfReservesHandler(c); err != nil {
		t.Fatal(err)
	}

	var resp CMCProofOfReserves
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}

	want := "https://gateway.liquify.com/chain/thorchain_api/thorchain/vaults/asgard"
	if resp.VerificationURL != want {
		t.Errorf("expected verification_url %s, got %s", want, resp.VerificationURL)
	}
}

// ---------------------------------------------------------------------------
// /proof-of-liabilities
// ---------------------------------------------------------------------------

func TestProofOfLiabilities_NoData(t *testing.T) {
	setTestThornodePools(nil)
	c, rec := newTestContext(http.MethodGet, "/thorchain/cmc/proof-of-liabilities")
	if err := CMCProofOfLiabilitiesHandler(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}

func TestProofOfLiabilities_SumsClaims(t *testing.T) {
	setTestThornodePools([]openapi.Pool{
		{
			Asset:        "BTC.BTC",
			BalanceAsset: "100000000", // 1 BTC pool depth
			SaversDepth:  "50000000",  // 0.5 BTC savers
			SynthSupply:  "25000000",  // 0.25 BTC synth
		},
	})

	c, rec := newTestContext(http.MethodGet, "/thorchain/cmc/proof-of-liabilities")
	if err := CMCProofOfLiabilitiesHandler(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp CMCProofOfLiabilities
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}

	if len(resp.Liabilities) != 1 {
		t.Fatalf("expected 1 liability, got %d", len(resp.Liabilities))
	}

	l := resp.Liabilities[0]
	if l.PoolDepth != "1.00000000" {
		t.Errorf("pool depth: %s", l.PoolDepth)
	}
	if l.SaversDepth != "0.50000000" {
		t.Errorf("savers depth: %s", l.SaversDepth)
	}
	if l.SynthSupply != "0.25000000" {
		t.Errorf("synth supply: %s", l.SynthSupply)
	}
	if l.TotalLiabilities != "1.75000000" {
		t.Errorf("total: %s", l.TotalLiabilities)
	}
	if l.Symbol != "BTC" || l.Chain != "BTC" {
		t.Errorf("symbol/chain wrong: %s / %s", l.Symbol, l.Chain)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func TestWalletAddressURL(t *testing.T) {
	tests := []struct {
		chain, addr, want string
	}{
		{"BTC", "bc1q123", "https://mempool.space/address/bc1q123"},
		{"ETH", "0xabc", "https://etherscan.io/address/0xabc"},
		{"SOL", "ABC123", "https://solscan.io/account/ABC123"},
		{"GAIA", "cosmos1abc", "https://www.mintscan.io/cosmos/address/cosmos1abc"},
		{"UNKNOWN", "addr", ""},
		{"BTC", "", ""},
	}
	for _, tt := range tests {
		got := walletAddressURL(tt.chain, tt.addr)
		if got != tt.want {
			t.Errorf("walletAddressURL(%q, %q) = %q, want %q", tt.chain, tt.addr, got, tt.want)
		}
	}
}

func TestFormatE8(t *testing.T) {
	if got := formatE8(100_000_000); got != "1.00000000" {
		t.Errorf("expected 1.00000000, got %s", got)
	}
	if got := formatE8(0); got != "0.00000000" {
		t.Errorf("expected 0.00000000, got %s", got)
	}
	if got := formatE8(123_456_789); got != "1.23456789" {
		t.Errorf("expected 1.23456789, got %s", got)
	}
}

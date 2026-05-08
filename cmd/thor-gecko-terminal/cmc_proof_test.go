package main

import (
	"encoding/json"
	"net/http"
	"testing"

	"cosmossdk.io/math"
	"gitlab.com/thorchain/thornode/v3/common"
	openapi "gitlab.com/thorchain/thornode/v3/openapi/gen"
	"gitlab.com/thorchain/thornode/v3/x/thorchain/types"
)

func setTestVaults(vaults []types.QueryVaultResponse) {
	thornodeThorchainVaultsAsgardMu.Lock()
	defer thornodeThorchainVaultsAsgardMu.Unlock()
	thornodeThorchainVaultsAsgard = vaults
}

func setTestThornodePools(pools []openapi.Pool) {
	thornodeThorchainPoolsMu.Lock()
	defer thornodeThorchainPoolsMu.Unlock()
	thornodeThorchainPools = pools
}

func makeTestVault(status string, height int64, addresses map[string]string, coins []common.Coin) types.QueryVaultResponse {
	addrPtrs := make([]*types.VaultAddress, 0, len(addresses))
	for chain, addr := range addresses {
		addrPtrs = append(addrPtrs, &types.VaultAddress{Chain: chain, Address: addr})
	}
	return types.QueryVaultResponse{
		BlockHeight: height,
		Status:      status,
		Coins:       coins,
		Addresses:   addrPtrs,
	}
}

func makeCoin(chain, ticker string, amount uint64) common.Coin {
	return common.Coin{
		Asset: common.Asset{
			Chain:  common.Chain(chain),
			Symbol: common.Symbol(ticker),
			Ticker: common.Ticker(ticker),
		},
		Amount: math.NewUint(amount),
	}
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
	setTestVaults([]types.QueryVaultResponse{
		makeTestVault("active", 100,
			map[string]string{"BTC": "bc1qaaa", "ETH": "0xaaa"},
			[]common.Coin{
				makeCoin("BTC", "BTC", 100_000_000),       // 1 BTC
				makeCoin("ETH", "ETH", 5_000_000_000),     // 50 ETH
			},
		),
		makeTestVault("active", 101,
			map[string]string{"BTC": "bc1qbbb", "ETH": "0xbbb"},
			[]common.Coin{
				makeCoin("BTC", "BTC", 200_000_000),       // 2 BTC
				makeCoin("ETH", "ETH", 1_000_000_000),     // 10 ETH
			},
		),
		makeTestVault("retiring", 99,
			map[string]string{"BTC": "bc1qccc"},
			[]common.Coin{
				makeCoin("BTC", "BTC", 999_000_000), // should be excluded
			},
		),
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

	// Find BTC reserve
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
	setTestVaults([]types.QueryVaultResponse{
		{
			BlockHeight: 100,
			Status:      "active",
			Coins: []common.Coin{
				makeCoin("BTC", "BTC", 100_000_000),
				{
					Asset: common.Asset{
						Chain:  "BTC",
						Symbol: "BTC",
						Ticker: "BTC",
						Synth:  true, // synthetic — should be excluded
					},
					Amount: math.NewUint(50_000_000),
				},
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

	// Should only have the L1 BTC entry, synth excluded.
	if len(resp.Reserves) != 1 {
		t.Fatalf("expected 1 reserve (synth excluded), got %d", len(resp.Reserves))
	}
	if resp.Reserves[0].TotalReserve != "1.00000000" {
		t.Errorf("expected synth-only excluded, got total %s", resp.Reserves[0].TotalReserve)
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

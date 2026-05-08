package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	openapi "gitlab.com/thorchain/thornode/v3/openapi/gen"
)

// thornodeAPIBase is the trimmed base URL for the configured THORNode HTTP API.
// Set by InitCache. Used to build verification URLs in handler responses.
var thornodeAPIBase string

////////////////////////////////////////////////////////////////////////////////////////
// Config
////////////////////////////////////////////////////////////////////////////////////////

const (
	ThornodeThorchainVaultsAsgardInterval = 60 * time.Second
	ThornodeThorchainMimirInterval        = 10 * time.Second
	ThornodeThorchainPoolsInterval        = 5 * time.Minute
	MidgardPoolsInterval                  = 60 * time.Second
)

////////////////////////////////////////////////////////////////////////////////////////
// <thornode>/thorchain/vaults/asgard
////////////////////////////////////////////////////////////////////////////////////////

// AsgardVault mirrors the relevant fields of a single vault entry returned by
// THORNode's /thorchain/vaults/asgard endpoint. The protobuf-generated
// QueryVaultResponse can't be JSON-decoded directly because nested types
// (Coin.Asset, Coin.Amount) use wire formats that don't match the HTTP API.
type AsgardVault struct {
	BlockHeight int64             `json:"block_height"`
	PubKey      string            `json:"pub_key"`
	Coins       []AsgardCoin      `json:"coins"`
	Type        string            `json:"type"`
	Status      string            `json:"status"`
	StatusSince int64             `json:"status_since"`
	Addresses   []AsgardVaultAddr `json:"addresses"`
}

// AsgardCoin is a single asset balance held by a vault.
type AsgardCoin struct {
	Asset    string `json:"asset"`              // e.g. "BTC.BTC", "ETH.USDT-0X..."
	Amount   string `json:"amount"`             // 1e8-scaled decimal as string
	Decimals int    `json:"decimals,omitempty"` // native decimals (optional)
}

// AsgardVaultAddr is a vault's wallet address on a particular chain.
type AsgardVaultAddr struct {
	Chain   string `json:"chain"`
	Address string `json:"address"`
}

var (
	thornodeThorchainVaultsAsgard   []AsgardVault
	thornodeThorchainVaultsAsgardMu sync.Mutex
)

func CachedThornodeThorchainVaultsAsgard() []AsgardVault {
	return thornodeThorchainVaultsAsgard
}

////////////////////////////////////////////////////////////////////////////////////////
// <thornode>/thorchain/mimir
////////////////////////////////////////////////////////////////////////////////////////

var (
	thornodeThorchainMimir   map[string]int64
	thornodeThorchainMimirMu sync.Mutex
)

func CachedThornodeThorchainMimir() map[string]int64 {
	return thornodeThorchainMimir
}

////////////////////////////////////////////////////////////////////////////////////////
// <thornode>/thorchain/pools
////////////////////////////////////////////////////////////////////////////////////////

var (
	thornodeThorchainPools   []openapi.Pool
	thornodeThorchainPoolsMu sync.RWMutex
)

func CachedThornodeThorchainPools() []openapi.Pool {
	thornodeThorchainPoolsMu.RLock()
	defer thornodeThorchainPoolsMu.RUnlock()
	// Return copy to prevent external modification
	pools := make([]openapi.Pool, len(thornodeThorchainPools))
	copy(pools, thornodeThorchainPools)
	return pools
}

////////////////////////////////////////////////////////////////////////////////////////
// <midgard>/v2/pools
////////////////////////////////////////////////////////////////////////////////////////

// MidgardPool mirrors the relevant fields from Midgard's /v2/pools endpoint.
type MidgardPool struct {
	Asset                string `json:"asset"`
	Status               string `json:"status"`
	AssetDepth           string `json:"assetDepth"`
	RuneDepth            string `json:"runeDepth"`
	AssetPrice           string `json:"assetPrice"`
	AssetPriceUSD        string `json:"assetPriceUSD"`
	Volume24h            string `json:"volume24h"`
	LiquidityInUSD       string `json:"liquidityInUSD"`
	NativeDecimal        string `json:"nativeDecimal"`
	AnnualPercentageRate string `json:"annualPercentageRate"`
	PoolAPY              string `json:"poolAPY"`
}

var (
	midgardPools   []MidgardPool
	midgardPoolsMu sync.RWMutex
)

func CachedMidgardPools() []MidgardPool {
	midgardPoolsMu.RLock()
	defer midgardPoolsMu.RUnlock()
	pools := make([]MidgardPool, len(midgardPools))
	copy(pools, midgardPools)
	return pools
}

////////////////////////////////////////////////////////////////////////////////////////
// Init
////////////////////////////////////////////////////////////////////////////////////////

func startCache(url string, response any, mu *sync.Mutex, interval time.Duration) {
	l := log.With().Str("url", url).Logger()

	update := func() error {
		l.Info().Msg("updating cache")
		res, err := http.Get(url)
		if err != nil {
			return err
		}
		defer res.Body.Close()
		if res.StatusCode != 200 {
			return err
		}
		if err := json.NewDecoder(res.Body).Decode(response); err != nil {
			return err
		}
		return nil
	}

	// attempt until success
	for {
		if err := update(); err != nil {
			l.Error().Err(err).Msg("failed to update cache")
			time.Sleep(time.Second)
			continue
		}
		break
	}

	// start ticker
	go func() {
		ticker := time.NewTicker(interval)
		for range ticker.C {
			err := update()
			if err != nil {
				l.Error().Err(err).Msg("failed to update cache")
			}
		}
	}()
}

func startCacheRW(url string, response any, mu *sync.RWMutex, interval time.Duration) {
	l := log.With().Str("url", url).Logger()

	update := func() error {
		l.Info().Msg("updating cache")
		res, err := http.Get(url)
		if err != nil {
			return err
		}
		defer res.Body.Close()
		if res.StatusCode != 200 {
			return err
		}

		mu.Lock()
		defer mu.Unlock()
		if err := json.NewDecoder(res.Body).Decode(response); err != nil {
			return err
		}
		return nil
	}

	// attempt until success
	for {
		if err := update(); err != nil {
			l.Error().Err(err).Msg("failed to update cache")
			time.Sleep(time.Second)
			continue
		}
		break
	}

	// start ticker
	go func() {
		ticker := time.NewTicker(interval)
		for range ticker.C {
			err := update()
			if err != nil {
				l.Error().Err(err).Msg("failed to update cache")
			}
		}
	}()
}

func InitCache(thornodeAPI, midgardAPI string) {
	thornodeAPIBase = strings.TrimRight(thornodeAPI, "/")

	startCache(
		// trunk-ignore(gitleaks/generic-api-key)
		fmt.Sprintf("%s/%s", thornodeAPI, "thorchain/vaults/asgard"),
		&thornodeThorchainVaultsAsgard,
		&thornodeThorchainVaultsAsgardMu,
		ThornodeThorchainVaultsAsgardInterval,
	)

	startCache(
		fmt.Sprintf("%s/%s", thornodeAPI, "thorchain/mimir"),
		&thornodeThorchainMimir,
		&thornodeThorchainMimirMu,
		ThornodeThorchainMimirInterval,
	)

	startCacheRW(
		fmt.Sprintf("%s/%s", thornodeAPI, "thorchain/pools"),
		&thornodeThorchainPools,
		&thornodeThorchainPoolsMu,
		ThornodeThorchainPoolsInterval,
	)

	startCacheRW(
		fmt.Sprintf("%s/%s", midgardAPI, "v2/pools"),
		&midgardPools,
		&midgardPoolsMu,
		MidgardPoolsInterval,
	)
}

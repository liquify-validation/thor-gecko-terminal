// CoinMarketCap (CMC) Proof of Reserves and Proof of Liabilities handlers.
//
// THORChain is non-custodial — all reserves are held in on-chain Asgard vaults
// controlled by the active validator set, and all liabilities (LP claims,
// savers, synths) are publicly visible on-chain. These endpoints aggregate
// that data into the format CMC expects for Annex J (PoR) and Annex K (PoL).

package main

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	echo "github.com/labstack/echo/v4"
)

// CMCProofOfReservesHandler returns Asgard vault balances per asset, with
// every vault address linked to a public block-explorer URL.
//
//	GET /thorchain/cmc/proof-of-reserves
func CMCProofOfReservesHandler(c echo.Context) error {
	vaults := CachedThornodeThorchainVaultsAsgard()
	if len(vaults) == 0 {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{
			"error": "vault data not available",
		})
	}

	// Aggregate balances per asset across all active vaults, and keep a list
	// of (vault address, balance) tuples for explorer-level verification.
	type vaultEntry struct {
		address string
		amount  uint64
	}
	type assetState struct {
		asset   string
		chain   string
		symbol  string
		total   uint64
		entries []vaultEntry
	}
	assets := make(map[string]*assetState)

	var maxBlockHeight int64
	for _, v := range vaults {
		if !strings.EqualFold(v.Status, "active") {
			continue
		}
		if v.BlockHeight > maxBlockHeight {
			maxBlockHeight = v.BlockHeight
		}

		// Build chain -> address lookup for this vault.
		chainAddr := make(map[string]string, len(v.Addresses))
		for _, a := range v.Addresses {
			if a != nil {
				chainAddr[a.Chain] = a.Address
			}
		}

		for _, coin := range v.Coins {
			// Skip synth/trade/secured derivative balances — they don't
			// represent physical reserves of the L1 asset.
			if coin.Asset.Synth || coin.Asset.Trade || coin.Asset.Secured {
				continue
			}

			assetID := coin.Asset.String()
			chain := string(coin.Asset.Chain)
			symbol := string(coin.Asset.Ticker)

			state, ok := assets[assetID]
			if !ok {
				state = &assetState{asset: assetID, chain: chain, symbol: symbol}
				assets[assetID] = state
			}
			amt := coin.Amount.Uint64()
			state.total += amt
			if addr := chainAddr[chain]; addr != "" {
				state.entries = append(state.entries, vaultEntry{address: addr, amount: amt})
			}
		}
	}

	// Sort assets alphabetically for stable output.
	assetIDs := make([]string, 0, len(assets))
	for id := range assets {
		assetIDs = append(assetIDs, id)
	}
	sort.Strings(assetIDs)

	reserves := make([]CMCReserve, 0, len(assetIDs))
	for _, id := range assetIDs {
		s := assets[id]

		vaultAddrs := make([]CMCVaultAddress, 0, len(s.entries))
		for _, e := range s.entries {
			vaultAddrs = append(vaultAddrs, CMCVaultAddress{
				Address:     e.address,
				Balance:     formatE8(e.amount),
				ExplorerURL: walletAddressURL(s.chain, e.address),
			})
		}

		reserves = append(reserves, CMCReserve{
			Asset:          s.asset,
			Symbol:         s.symbol,
			Chain:          s.chain,
			TotalReserve:   formatE8(s.total),
			VaultAddresses: vaultAddrs,
		})
	}

	return c.JSON(http.StatusOK, CMCProofOfReserves{
		Network:         "thorchain",
		BlockHeight:     maxBlockHeight,
		Timestamp:       time.Now().Unix(),
		VerificationURL: "https://thornode.ninerealms.com/thorchain/vaults/asgard",
		Reserves:        reserves,
	})
}

// CMCProofOfLiabilitiesHandler returns per-pool LP claims, savers deposits,
// and synth supply — the outstanding claims against vault reserves.
//
//	GET /thorchain/cmc/proof-of-liabilities
func CMCProofOfLiabilitiesHandler(c echo.Context) error {
	pools := CachedThornodeThorchainPools()
	if len(pools) == 0 {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{
			"error": "pool data not available",
		})
	}

	liabilities := make([]CMCLiability, 0, len(pools))
	for _, p := range pools {
		chain, sym := extractChainSymbol(p.Asset)

		poolDepth := parseIntOrZero(p.BalanceAsset)
		savers := parseIntOrZero(p.SaversDepth)
		synth := parseIntOrZero(p.SynthSupply)
		total := poolDepth + savers + synth

		liabilities = append(liabilities, CMCLiability{
			Asset:            p.Asset,
			Symbol:           sym,
			Chain:            chain,
			PoolDepth:        formatE8(uint64(poolDepth)),
			SaversDepth:      formatE8(uint64(savers)),
			SynthSupply:      formatE8(uint64(synth)),
			TotalLiabilities: formatE8(uint64(total)),
		})
	}

	sort.Slice(liabilities, func(i, j int) bool {
		return liabilities[i].Asset < liabilities[j].Asset
	})

	return c.JSON(http.StatusOK, CMCProofOfLiabilities{
		Network:     "thorchain",
		Timestamp:   time.Now().Unix(),
		Liabilities: liabilities,
	})
}

// formatE8 converts a 1e8 integer amount to a fixed-precision decimal string.
func formatE8(amount uint64) string {
	return fmt.Sprintf("%.8f", float64(amount)/1e8)
}

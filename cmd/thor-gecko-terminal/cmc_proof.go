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
	"strconv"
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
		if !strings.EqualFold(v.Status, "ActiveVault") {
			continue
		}
		if v.BlockHeight > maxBlockHeight {
			maxBlockHeight = v.BlockHeight
		}

		// Build chain -> address lookup for this vault.
		chainAddr := make(map[string]string, len(v.Addresses))
		for _, a := range v.Addresses {
			chainAddr[a.Chain] = a.Address
		}

		for _, coin := range v.Coins {
			// Skip synth (CHAIN/SYMBOL) and trade (CHAIN~SYMBOL) derivatives —
			// only physical L1 reserves count toward proof-of-reserves.
			if strings.ContainsAny(coin.Asset, "/~") {
				continue
			}

			chain, symbol := extractChainSymbol(coin.Asset)
			amount, err := strconv.ParseUint(coin.Amount, 10, 64)
			if err != nil || amount == 0 {
				continue
			}

			state, ok := assets[coin.Asset]
			if !ok {
				state = &assetState{asset: coin.Asset, chain: chain, symbol: symbol}
				assets[coin.Asset] = state
			}
			state.total += amount
			if addr := chainAddr[chain]; addr != "" {
				state.entries = append(state.entries, vaultEntry{address: addr, amount: amount})
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

	verificationURL := thornodeAPIBase + "/thorchain/vaults/asgard"

	return c.JSON(http.StatusOK, CMCProofOfReserves{
		Network:         "thorchain",
		BlockHeight:     maxBlockHeight,
		Timestamp:       time.Now().Unix(),
		VerificationURL: verificationURL,
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

# THORChain DEX Integration API

A Go service that exposes THORChain's swap, liquidity, and pricing data through two industry-standard APIs:

- **[GeckoTerminal DEX Integration](https://docs.google.com/document/d/1ufjAJUa6rGO9PBGJGwfBMn-XMk9NE0ow3_iMYrS3drk/edit?tab=t.0)** — block, asset, pair, and event endpoints used by GeckoTerminal's indexer
- **[CoinMarketCap Ideal API](https://docs.google.com/document/d/1S4urpzUnO2t7DmS_1dc4EL4tgnnbTObPYXvDeBnukCg/)** — spot summary/ticker/trades plus DEX subgraph-style swaps used by CMC

## API documentation

Interactive Swagger UI is served alongside the API:

| Path | Description |
|---|---|
| `/docs` | Swagger UI (interactive browser explorer) |
| `/openapi.yaml` | OpenAPI 3.0 spec (machine-readable) |

The spec is embedded in the binary via `go:embed`, so no separate documentation deployment is needed — it ships with the service.

## Endpoints

### GeckoTerminal

| Endpoint | Description |
|----------|-------------|
| `GET /thorchain/geckoterminal/latest-block` | Latest indexed block number and timestamp |
| `GET /thorchain/geckoterminal/asset?id=:id` | Asset info (name, symbol, decimals, supply, CoinGecko ID) |
| `GET /thorchain/geckoterminal/pair?id=:id` | Trading pair info (RUNE + asset, fees) |
| `GET /thorchain/geckoterminal/events?fromBlock=:n&toBlock=:n` | Swap and liquidity events for a block range |

### CoinMarketCap

| Endpoint | Description |
|----------|-------------|
| `GET /thorchain/cmc/summary` | Overview of every market with 24h volume, last price, depth |
| `GET /thorchain/cmc/assets` | Asset metadata (contract address, explorer link, fees) keyed by symbol |
| `GET /thorchain/cmc/ticker` | Compact 24h pricing/volume map keyed by trading pair |
| `GET /thorchain/cmc/trades?market_pair=RUNE_BTC` | Recent swap trades for a market pair |
| `GET /thorchain/cmc/swaps?limit=100` | Recent swaps in DEX subgraph (C2) format |
| `GET /thorchain/cmc/orderbook?market_pair=RUNE_BTC&depth=100&level=2` | Synthesized level-2 orderbook from the AMM bonding curve |
| `GET /thorchain/cmc/proof-of-reserves` | Per-asset Asgard vault balances with on-chain explorer links (CMC Annex J) |
| `GET /thorchain/cmc/proof-of-liabilities` | Per-asset claims (pool depth, savers, synth supply) outstanding against vault reserves (CMC Annex K) |

Trading pairs use the format `RUNE_<symbol>` (e.g. `RUNE_BTC`, `RUNE_ETH`). When a symbol exists on multiple chains (e.g. USDC on ETH, BSC, AVAX, BASE), the chain is appended to disambiguate: `RUNE_USDC-ETH`, `RUNE_USDC-BSC`, etc.

#### A note on the orderbook

THORChain has no native orderbook — it's a constant-product AMM. The `/orderbook` endpoint synthesizes a level-2 book by discretizing the bonding curve at fixed percentage steps from spot price. Each level represents the *incremental* RUNE quantity you'd need to push the pool price to that level via a single trade.

Query parameters:
- `market_pair` (required): e.g. `RUNE_BTC`
- `depth` (optional): total entries across both sides — `[5, 10, 20, 50, 100, 500]`. Defaults to 100 (50 each side).
- `level` (optional): `1` = best bid/ask only, `2` = arranged levels (default), `3` = deep book.

Note: prices reflect the pure bonding curve and don't include THORChain's slip-based fees, which are surfaced separately via `/thorchain/geckoterminal/pair`.

#### A note on Proof of Reserves

THORChain is non-custodial — there is no centralized treasury. All reserves are held in on-chain Asgard vaults (multi-signature wallets controlled by the active validator set), and all liabilities (LP units, savers, synth supply) are recorded on-chain. The `/proof-of-reserves` endpoint exposes vault balances per asset along with the actual block-explorer URLs for every vault address, so reserves can be independently audited at any block height. For a solvent network, vault balance ≥ pool depth + savers + synth supply for each asset.

## Architecture

- **THORNode cache** — Polls THORNode for pool, mimir, and vault data
- **Midgard cache** — Polls Midgard's `/v2/pools` for 24h volume and price data
- **Midgard DB** — Queries a Midgard PostgreSQL database for historical block, swap, and liquidity events
- **Event processing** — Parallel queries with in-memory pool depth lookups via binary search

## Prerequisites

- Go 1.24+
- Access to a [Midgard](https://gitlab.com/thorchain/midgard) PostgreSQL database (for `/events` and `/trades`)
- Network access to a THORNode API and Midgard HTTP API

## Configuration

| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `THORNODE_API` | `https://gateway.liquify.com/chain/thorchain_api/` | THORNode API base URL |
| `MIDGARD_API` | `https://gateway.liquify.com/chain/thorchain_midgard` | Midgard HTTP API base URL |
| `MIDGARD_DSN` | `host=localhost port=5432 user=midgard dbname=midgard password=password sslmode=disable` | Midgard PostgreSQL connection string |

## Run locally

```bash
go build -o gecko-terminal ./cmd/thor-gecko-terminal
./gecko-terminal
```

The server starts on port **1323**.

## Deploy with Docker

```bash
docker compose up -d --build
```

This starts:
- **gecko-terminal** on port 1323
- **midgard-db** (PostgreSQL 16) on port 5432

> **Note:** The database starts empty. You need to either populate it with a Midgard indexer, or point `MIDGARD_DSN` at an existing Midgard database and remove the `midgard-db` service from `docker-compose.yml`.

## Tests

Run the test suite (no database required):

```bash
go test ./cmd/thor-gecko-terminal/ -v
```

Test files:

- **`handlers_test.go`** — GeckoTerminal endpoint tests (latest-block, asset, pair, events)
- **`events_test.go`** — Event transform and helper tests (swap direction, price fallback, e8 conversion, asset parsing, pool depth binary search)
- **`cmc_test.go`** — CMC endpoint tests (summary, assets, ticker, trades, swaps) plus trading-pair collision handling and contract URL resolution
- **`cmc_orderbook_test.go`** — Synthesized orderbook tests (bid/ask ordering, constant-product preservation, depth/level resolution, edge cases)
- **`cmc_proof_test.go`** — Proof of Reserves / Proof of Liabilities tests (vault aggregation, synth exclusion, claims math, wallet explorer URLs)

## Project structure

```
cmd/thor-gecko-terminal/
  main.go            — Server startup and route registration
  cache.go           — THORNode and Midgard data caches
  types.go           — GeckoTerminal types, CoinGecko/asset mappings
  handlers.go        — GeckoTerminal HTTP handlers
  events.go          — GeckoTerminal event transformation and pricing
  midgard.go         — Database init, event queries, pool depth lookups
  cmc_types.go       — CoinMarketCap API response types
  cmc.go             — CMC helpers (pair IDs, asset parsing, explorer URLs)
  cmc_handlers.go    — CoinMarketCap HTTP handlers (summary, assets, ticker, trades, swaps)
  cmc_orderbook.go   — Synthesized orderbook from the AMM bonding curve
  cmc_proof.go       — Proof of Reserves / Proof of Liabilities handlers
  docs.go            — OpenAPI spec embedding + Swagger UI handler
  openapi.yaml       — OpenAPI 3.0 specification (embedded into the binary)
  *_test.go          — Test files
Dockerfile           — Multi-stage build
docker-compose.yml
```

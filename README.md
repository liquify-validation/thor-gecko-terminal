# GeckoTerminal THORChain Integration API

A Go service that implements the [GeckoTerminal DEX Integration API](https://docs.google.com/document/d/1ufjAJUa6rGO9PBGJGwfBMn-XMk9NE0ow3_iMYrS3drk/edit?tab=t.0) for THORChain, enabling GeckoTerminal to track real-time and historical swap, liquidity, and pricing data.

## Endpoints

| Endpoint | Description |
|----------|-------------|
| `GET /thorchain/geckoterminal/latest-block` | Latest indexed block number and timestamp |
| `GET /thorchain/geckoterminal/asset?id=:id` | Asset info (name, symbol, decimals, supply, CoinGecko ID) |
| `GET /thorchain/geckoterminal/pair?id=:id` | Trading pair info (RUNE + asset, fees) |
| `GET /thorchain/geckoterminal/events?fromBlock=:n&toBlock=:n` | Swap and liquidity events for a block range |

## Architecture

- **THORNode cache** — Polls THORNode for pool, mimir, and vault data on intervals
- **Midgard DB** — Queries a Midgard PostgreSQL database for historical block, swap, and liquidity events
- **Event processing** — Parallel queries with in-memory pool depth lookups via binary search

## Prerequisites

- Go 1.24+
- Access to a [Midgard](https://gitlab.com/thorchain/midgard) PostgreSQL database
- Network access to a THORNode API

## Configuration

| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `THORNODE_API` | `https://gateway.liquify.com/chain/thorchain_api/` | THORNode API base URL |
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

Tests cover all four endpoints and the core transform logic:

- **Handler tests** (`handlers_test.go`) — HTTP-level tests for `/latest-block`, `/asset`, `/pair`, and `/events` including error cases (missing params, empty cache, no DB), RUNE synthetic responses, known/unknown pools, contract address parsing, and fee defaults
- **Event tests** (`events_test.go`) — Unit tests for swap/liquidity transforms, price calculation with reserve fallback, `e8ToDecimal` conversion, `parseAssetID`, and `findPoolDepth` binary search

## Project structure

```
cmd/thor-gecko-terminal/
  main.go            — Server startup and route registration
  types.go           — API response types, CoinGecko/asset mappings
  handlers.go        — HTTP handler functions
  handlers_test.go   — Endpoint tests
  midgard.go         — Database init, event queries, pool depth lookups
  events.go          — Event transformation and price calculation
  events_test.go     — Transform and helper tests
  cache.go           — THORNode data cache (pools, mimir, vaults)
Dockerfile           — Multi-stage build
docker-compose.yml
```

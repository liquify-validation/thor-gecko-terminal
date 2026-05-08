package main

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "github.com/lib/pq"
	"github.com/rs/zerolog/log"
)

var midgardDB *sql.DB

func initMidgardDB() {
	connStr := os.Getenv("MIDGARD_DSN")
	if connStr == "" {
		connStr = "host=localhost port=5432 user=midgard dbname=midgard password=password sslmode=disable"
	}

	var err error
	midgardDB, err = sql.Open("postgres", connStr)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to midgard database")
	}

	midgardDB.SetMaxOpenConns(8)
	midgardDB.SetMaxIdleConns(8)
	midgardDB.SetConnMaxLifetime(5 * time.Minute)

	if err := midgardDB.Ping(); err != nil {
		log.Warn().Err(err).Msg("midgard database connection failed - events endpoint will not work")
		midgardDB = nil
	} else {
		log.Info().Msg("connected to midgard database")
	}
}

// queryEvents fetches swap, join, and exit events for the given block range
// and returns them as GeckoTerminal-formatted events.
func queryEvents(ctx context.Context, fromBlock, toBlock int64) ([]interface{}, error) {
	// Resolve block heights to timestamps.
	startTs, endTs, err := resolveBlockTimestamps(ctx, fromBlock, toBlock)
	if err != nil {
		return nil, err
	}
	if startTs == 0 && endTs == 0 {
		return []interface{}{}, nil
	}

	// Fetch raw events from all three tables in parallel.
	allEvents, err := fetchRawEvents(ctx, startTs, endTs)
	if err != nil {
		return nil, err
	}

	// Sort by timestamp then event ID for consistent ordering.
	sort.Slice(allEvents, func(i, j int) bool {
		if allEvents[i].BlockTimestamp == allEvents[j].BlockTimestamp {
			return allEvents[i].EventID < allEvents[j].EventID
		}
		return allEvents[i].BlockTimestamp < allEvents[j].BlockTimestamp
	})

	// Group events by block (seconds-level key).
	blockEvents := groupEventsByBlock(allEvents)

	// Fetch pool depth snapshots for reserve lookups.
	depthLookup, err := buildPoolDepthLookup(ctx, allEvents)
	if err != nil {
		return nil, err
	}

	// Assign indices and transform into API response events.
	return assembleEvents(allEvents, blockEvents, depthLookup)
}

// resolveBlockTimestamps converts block heights to Midgard nanosecond timestamps.
func resolveBlockTimestamps(ctx context.Context, fromBlock, toBlock int64) (int64, int64, error) {
	query := `
		SELECT
			(SELECT timestamp FROM midgard.block_log WHERE height = $1) AS start_ts,
			(SELECT timestamp FROM midgard.block_log WHERE height = $2) AS end_ts
	`
	var startTs, endTs sql.NullInt64
	err := midgardDB.QueryRowContext(ctx, query, fromBlock, toBlock).Scan(&startTs, &endTs)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get timestamp range: %w", err)
	}
	if !startTs.Valid || !endTs.Valid {
		return 0, 0, nil
	}
	return startTs.Int64, endTs.Int64, nil
}

// fetchRawEvents queries swaps, stakes, and withdrawals in parallel.
func fetchRawEvents(ctx context.Context, startTs, endTs int64) ([]rawEvent, error) {
	var allEvents []rawEvent
	var mu sync.Mutex
	var wg sync.WaitGroup
	errCh := make(chan error, 3)

	wg.Add(3)
	go fetchSwapEvents(ctx, startTs, endTs, &allEvents, &mu, &wg, errCh)
	go fetchStakeEvents(ctx, startTs, endTs, &allEvents, &mu, &wg, errCh)
	go fetchWithdrawEvents(ctx, startTs, endTs, &allEvents, &mu, &wg, errCh)

	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			return nil, err
		}
	}
	return allEvents, nil
}

func fetchSwapEvents(ctx context.Context, startTs, endTs int64, dest *[]rawEvent, mu *sync.Mutex, wg *sync.WaitGroup, errCh chan<- error) {
	defer wg.Done()
	query := `
		SELECT 'swap', s.tx, s.from_addr, s.pool,
			s.from_asset, s.from_e8, s.to_asset, s.to_e8,
			'', 0::bigint, 0::bigint, 0::bigint, 0::bigint,
			s.block_timestamp, s.event_id, bl.height
		FROM midgard.swap_events s
		JOIN midgard.block_log bl ON bl.timestamp = s.block_timestamp
		WHERE s.block_timestamp BETWEEN $1 AND $2
		ORDER BY s.block_timestamp ASC, s.event_id ASC
	`
	events, err := scanRawEvents(ctx, query, startTs, endTs)
	if err != nil {
		errCh <- fmt.Errorf("swap events: %w", err)
		return
	}
	mu.Lock()
	*dest = append(*dest, events...)
	mu.Unlock()
}

func fetchStakeEvents(ctx context.Context, startTs, endTs int64, dest *[]rawEvent, mu *sync.Mutex, wg *sync.WaitGroup, errCh chan<- error) {
	defer wg.Done()
	query := `
		SELECT 'join', COALESCE(st.rune_tx, st.asset_tx), COALESCE(st.rune_addr, st.asset_addr, ''), st.pool,
			'', 0::bigint, '', 0::bigint,
			st.pool, st.asset_e8, st.rune_e8, 0::bigint, 0::bigint,
			st.block_timestamp, st.event_id, bl.height
		FROM midgard.stake_events st
		JOIN midgard.block_log bl ON bl.timestamp = st.block_timestamp
		WHERE st.block_timestamp BETWEEN $1 AND $2
		  AND (st.memo LIKE 'ADD:%' OR st.memo LIKE '+:%' OR st.memo LIKE 'a:%')
		ORDER BY st.block_timestamp ASC, st.event_id ASC
	`
	events, err := scanRawEvents(ctx, query, startTs, endTs)
	if err != nil {
		errCh <- fmt.Errorf("stake events: %w", err)
		return
	}
	mu.Lock()
	*dest = append(*dest, events...)
	mu.Unlock()
}

func fetchWithdrawEvents(ctx context.Context, startTs, endTs int64, dest *[]rawEvent, mu *sync.Mutex, wg *sync.WaitGroup, errCh chan<- error) {
	defer wg.Done()
	query := `
		SELECT 'exit', w.tx, w.from_addr, w.pool,
			'', 0::bigint, '', 0::bigint,
			w.asset, 0::bigint, 0::bigint, w.emit_asset_e8, w.emit_rune_e8,
			w.block_timestamp, w.event_id, bl.height
		FROM midgard.withdraw_events w
		JOIN midgard.block_log bl ON bl.timestamp = w.block_timestamp
		WHERE w.block_timestamp BETWEEN $1 AND $2
		  AND (w.memo LIKE '-:%' OR w.memo LIKE 'WITHDRAW:%' OR w.memo LIKE 'wd:%' OR w.memo LIKE 'WD:%')
		ORDER BY w.block_timestamp ASC, w.event_id ASC
	`
	events, err := scanRawEvents(ctx, query, startTs, endTs)
	if err != nil {
		errCh <- fmt.Errorf("withdraw events: %w", err)
		return
	}
	mu.Lock()
	*dest = append(*dest, events...)
	mu.Unlock()
}

// scanRawEvents executes a query and scans the result rows into rawEvent slices.
func scanRawEvents(ctx context.Context, query string, startTs, endTs int64) ([]rawEvent, error) {
	rows, err := midgardDB.QueryContext(ctx, query, startTs, endTs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []rawEvent
	for rows.Next() {
		var e rawEvent
		err := rows.Scan(
			&e.EventType, &e.TxnID, &e.Maker, &e.PairID,
			&e.FromAsset, &e.FromE8, &e.ToAsset, &e.ToE8,
			&e.Asset, &e.AssetE8, &e.RuneE8, &e.EmitAssetE8, &e.EmitRuneE8,
			&e.BlockTimestamp, &e.EventID, &e.BlockNumber,
		)
		if err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, nil
}

// groupEventsByBlock groups event indices by block timestamp (seconds).
func groupEventsByBlock(events []rawEvent) map[int64][]int {
	blocks := make(map[int64][]int)
	for i, e := range events {
		blockSec := e.BlockTimestamp / 1_000_000_000
		blocks[blockSec] = append(blocks[blockSec], i)
	}
	return blocks
}

// buildPoolDepthLookup fetches pool depth snapshots and builds an efficient
// lookup map keyed by pool name with depths sorted DESC by timestamp.
func buildPoolDepthLookup(ctx context.Context, events []rawEvent) (map[string][]poolDepth, error) {
	// Collect unique pools and timestamp bounds.
	poolSet := make(map[string]bool)
	var minTs, maxTs int64 = math.MaxInt64, 0
	for _, e := range events {
		if e.PairID == "" {
			continue
		}
		poolSet[e.PairID] = true
		if e.BlockTimestamp < minTs {
			minTs = e.BlockTimestamp
		}
		if e.BlockTimestamp > maxTs {
			maxTs = e.BlockTimestamp
		}
	}

	if len(poolSet) == 0 {
		return nil, nil
	}

	pools := make([]string, 0, len(poolSet))
	for p := range poolSet {
		pools = append(pools, p)
	}

	// Buffer = max(1 hour, 2x query duration) to handle pools that update infrequently.
	queryDuration := maxTs - minTs
	minBuffer := int64(3600 * 1_000_000_000)
	dynamicBuffer := queryDuration * 2
	timeBuffer := minBuffer
	if dynamicBuffer > minBuffer {
		timeBuffer = dynamicBuffer
	}

	query := `
		SELECT pool, block_timestamp, rune_e8, asset_e8
		FROM midgard.block_pool_depths
		WHERE pool = ANY($1::text[])
		  AND block_timestamp BETWEEN $2 AND $3
		ORDER BY pool, block_timestamp DESC
	`
	poolArray := "{" + strings.Join(pools, ",") + "}"

	rows, err := midgardDB.QueryContext(ctx, query, poolArray, minTs-timeBuffer, maxTs)
	if err != nil {
		return nil, fmt.Errorf("failed to query pool depths: %w", err)
	}
	defer rows.Close()

	lookup := make(map[string][]poolDepth)
	for rows.Next() {
		var d poolDepth
		if err := rows.Scan(&d.Pool, &d.BlockTimestamp, &d.RuneE8, &d.AssetE8); err != nil {
			return nil, fmt.Errorf("failed to scan pool depth: %w", err)
		}
		lookup[d.Pool] = append(lookup[d.Pool], d)
	}
	return lookup, nil
}

// findPoolDepth returns the most recent pool reserves at or before the given
// timestamp using binary search. Depths are sorted DESC by timestamp.
func findPoolDepth(lookup map[string][]poolDepth, pool string, timestamp int64) (runeE8, assetE8 int64) {
	depths, ok := lookup[pool]
	if !ok {
		return 0, 0
	}

	left, right := 0, len(depths)-1
	result := -1
	for left <= right {
		mid := (left + right) / 2
		if depths[mid].BlockTimestamp <= timestamp {
			result = mid
			right = mid - 1
		} else {
			left = mid + 1
		}
	}

	if result != -1 {
		return depths[result].RuneE8, depths[result].AssetE8
	}
	return 0, 0
}

// assembleEvents assigns transaction/event indices and transforms raw events
// into GeckoTerminal API response format.
func assembleEvents(allEvents []rawEvent, blockEvents map[int64][]int, depthLookup map[string][]poolDepth) ([]interface{}, error) {
	// Sort blocks chronologically for deterministic output.
	sortedBlocks := make([]int64, 0, len(blockEvents))
	for blockTs := range blockEvents {
		sortedBlocks = append(sortedBlocks, blockTs)
	}
	sort.Slice(sortedBlocks, func(i, j int) bool { return sortedBlocks[i] < sortedBlocks[j] })

	events := make([]interface{}, 0)

	for _, blockTs := range sortedBlocks {
		indices := blockEvents[blockTs]

		// Sort by txn ID for deterministic transaction index assignment.
		sort.Slice(indices, func(i, j int) bool {
			return allEvents[indices[i]].TxnID < allEvents[indices[j]].TxnID
		})

		// Assign transaction indices.
		txnIndexMap := make(map[string]int)
		txnCounter := 0
		for _, idx := range indices {
			if _, exists := txnIndexMap[allEvents[idx].TxnID]; !exists {
				txnIndexMap[allEvents[idx].TxnID] = txnCounter
				txnCounter++
			}
		}

		// Sort by (txnID, eventID) for event index assignment within transactions.
		sort.Slice(indices, func(i, j int) bool {
			ei, ej := allEvents[indices[i]], allEvents[indices[j]]
			if ei.TxnID == ej.TxnID {
				return ei.EventID < ej.EventID
			}
			return ei.TxnID < ej.TxnID
		})

		eventCounters := make(map[string]int)
		for _, idx := range indices {
			e := &allEvents[idx]
			txnIndex := txnIndexMap[e.TxnID]
			eventIndex := eventCounters[e.TxnID]
			eventCounters[e.TxnID]++

			poolRuneE8, poolAssetE8 := findPoolDepth(depthLookup, e.PairID, e.BlockTimestamp)

			block := Block{
				BlockNumber:    e.BlockNumber,
				BlockTimestamp: e.BlockTimestamp / 1_000_000_000,
			}

			switch e.EventType {
			case "swap":
				transformed, valid := transformSwapEvent(block, e, txnIndex, eventIndex, poolRuneE8, poolAssetE8)
				if valid {
					events = append(events, transformed)
				}
			case "join", "exit":
				events = append(events, transformLiquidityEvent(block, e, txnIndex, eventIndex, poolRuneE8, poolAssetE8))
			}
		}
	}

	return events, nil
}

////////////////////////////////////////////////////////////////////////////////////////
// CMC trade/swap queries
////////////////////////////////////////////////////////////////////////////////////////

// queryRecentTrades returns the most recent N swap trades for a given pool
// formatted as CMC trades. RUNE is the base currency, the pool asset is the quote.
func queryRecentTrades(ctx context.Context, asset string, limit int) ([]CMCTrade, error) {
	query := `
		SELECT s.tx, s.from_asset, s.from_e8, s.to_asset, s.to_e8, s.block_timestamp
		FROM midgard.swap_events s
		WHERE s.pool = $1
		ORDER BY s.block_timestamp DESC, s.event_id DESC
		LIMIT $2
	`
	rows, err := midgardDB.QueryContext(ctx, query, asset, limit)
	if err != nil {
		return nil, fmt.Errorf("query trades: %w", err)
	}
	defer rows.Close()

	trades := make([]CMCTrade, 0, limit)
	for rows.Next() {
		var (
			txID       string
			fromAsset  string
			fromE8     int64
			toAsset    string
			toE8       int64
			blockNanos int64
		)
		if err := rows.Scan(&txID, &fromAsset, &fromE8, &toAsset, &toE8, &blockNanos); err != nil {
			return nil, fmt.Errorf("scan trade: %w", err)
		}

		trade := buildTradeEntry(txID, fromAsset, fromE8, toAsset, toE8, blockNanos)
		if trade != nil {
			trades = append(trades, *trade)
		}
	}
	return trades, nil
}

// buildTradeEntry converts a single swap row into a CMCTrade. Returns nil if
// the swap doesn't involve RUNE (which should never happen in practice).
//
// RUNE is the base. So a RUNE→Asset swap is a "buy" of asset (sell of RUNE),
// and an Asset→RUNE swap is a "sell" of asset (buy of RUNE). The CMC `type`
// field describes the order that was matched on a traditional book — which is
// not directly applicable to AMMs, but we use the convention that Asset→RUNE
// = "buy" (RUNE was bought) and RUNE→Asset = "sell" (RUNE was sold).
func buildTradeEntry(txID, fromAsset string, fromE8 int64, toAsset string, toE8 int64, blockNanos int64) *CMCTrade {
	timestampMs := blockNanos / 1_000_000

	switch {
	case fromAsset == "THOR.RUNE":
		// Sell RUNE for asset.
		baseVol := float64(fromE8) / 1e8
		quoteVol := float64(toE8) / 1e8
		if baseVol == 0 {
			return nil
		}
		return &CMCTrade{
			TradeID:     txID,
			Price:       quoteVol / baseVol,
			BaseVolume:  baseVol,
			QuoteVolume: quoteVol,
			Timestamp:   timestampMs,
			Type:        "sell",
		}
	case toAsset == "THOR.RUNE":
		// Buy RUNE with asset.
		baseVol := float64(toE8) / 1e8
		quoteVol := float64(fromE8) / 1e8
		if baseVol == 0 {
			return nil
		}
		return &CMCTrade{
			TradeID:     txID,
			Price:       quoteVol / baseVol,
			BaseVolume:  baseVol,
			QuoteVolume: quoteVol,
			Timestamp:   timestampMs,
			Type:        "buy",
		}
	}
	return nil
}

// queryRecentSwaps returns the most recent N swaps in subgraph-style format
// (DEX section C2). Includes asset symbol and decimals.
func queryRecentSwaps(ctx context.Context, limit int) ([]CMCSwap, error) {
	query := `
		SELECT s.tx, s.from_asset, s.from_e8, s.to_asset, s.to_e8, s.block_timestamp
		FROM midgard.swap_events s
		ORDER BY s.block_timestamp DESC, s.event_id DESC
		LIMIT $1
	`
	rows, err := midgardDB.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("query swaps: %w", err)
	}
	defer rows.Close()

	swaps := make([]CMCSwap, 0, limit)
	for rows.Next() {
		var (
			txID       string
			fromAsset  string
			fromE8     int64
			toAsset    string
			toE8       int64
			blockNanos int64
		)
		if err := rows.Scan(&txID, &fromAsset, &fromE8, &toAsset, &toE8, &blockNanos); err != nil {
			return nil, fmt.Errorf("scan swap: %w", err)
		}

		_, fromSym := extractChainSymbol(fromAsset)
		_, toSym := extractChainSymbol(toAsset)

		swaps = append(swaps, CMCSwap{
			ID:         txID,
			FromAmount: strconv.FormatInt(fromE8, 10),
			ToAmount:   strconv.FormatInt(toE8, 10),
			Timestamp:  blockNanos / 1_000_000_000,
			Pair: CMCSwapPair{
				FromToken: CMCSwapToken{Decimals: 8, Symbol: fromSym},
				ToToken:   CMCSwapToken{Decimals: 8, Symbol: toSym},
			},
		})
	}
	return swaps, nil
}

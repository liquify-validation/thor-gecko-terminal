package main

import (
	"testing"
)

func TestE8ToDecimal(t *testing.T) {
	tests := []struct {
		input    int64
		expected string
	}{
		{100000000, "1.00000000"},
		{0, "0.00000000"},
		{1, "0.00000001"},
		{50000000, "0.50000000"},
		{123456789, "1.23456789"},
	}
	for _, tt := range tests {
		got := e8ToDecimal(tt.input)
		if got != tt.expected {
			t.Errorf("e8ToDecimal(%d) = %s, want %s", tt.input, got, tt.expected)
		}
	}
}

func TestCalculateReservePrice(t *testing.T) {
	// Normal case
	price := calculateReservePrice(100000000, 200000000)
	if price == "" {
		t.Fatal("expected non-empty price")
	}

	// Zero rune
	if got := calculateReservePrice(0, 200000000); got != "" {
		t.Fatalf("expected empty for zero rune, got %s", got)
	}

	// Zero asset
	if got := calculateReservePrice(100000000, 0); got != "" {
		t.Fatalf("expected empty for zero asset, got %s", got)
	}

	// Both zero
	if got := calculateReservePrice(0, 0); got != "" {
		t.Fatalf("expected empty for both zero, got %s", got)
	}
}

func TestIsValidPrice(t *testing.T) {
	if !isValidPrice(1.5) {
		t.Fatal("1.5 should be valid")
	}
	if !isValidPrice(0.000001) {
		t.Fatal("small positive should be valid")
	}
	if isValidPrice(0) {
		t.Fatal("0 should be invalid")
	}
	if isValidPrice(-1) {
		t.Fatal("negative should be invalid")
	}
	if isValidPrice(1e16) {
		t.Fatal("extremely large should be invalid")
	}
}

func TestTransformSwapEvent_RuneToAsset(t *testing.T) {
	block := Block{BlockNumber: 100, BlockTimestamp: 1698126147}
	e := &rawEvent{
		TxnID:     "TXHASH1",
		Maker:     "thor1abc",
		PairID:    "BTC.BTC",
		FromAsset: "THOR.RUNE",
		FromE8:    1000000000, // 10 RUNE
		ToAsset:   "BTC.BTC",
		ToE8:      5000000, // 0.05 BTC
	}

	event, valid := transformSwapEvent(block, e, 0, 0, 500000000000, 250000000)
	if !valid {
		t.Fatal("expected valid swap event")
	}
	if event.EventType != "swap" {
		t.Fatalf("expected eventType swap, got %s", event.EventType)
	}
	if event.Asset0In == nil {
		t.Fatal("expected asset0In to be set for RUNE->Asset")
	}
	if event.Asset1Out == nil {
		t.Fatal("expected asset1Out to be set for RUNE->Asset")
	}
	if event.Asset1In != nil {
		t.Fatal("expected asset1In to be nil for RUNE->Asset")
	}
	if event.Asset0Out != nil {
		t.Fatal("expected asset0Out to be nil for RUNE->Asset")
	}
	if event.PriceNative == "" || event.PriceNative == "0" {
		t.Fatalf("expected valid priceNative, got %s", event.PriceNative)
	}
	if event.PairID != "BTC.BTC" {
		t.Fatalf("expected pairId BTC.BTC, got %s", event.PairID)
	}
}

func TestTransformSwapEvent_AssetToRune(t *testing.T) {
	block := Block{BlockNumber: 101, BlockTimestamp: 1698126200}
	e := &rawEvent{
		TxnID:     "TXHASH2",
		Maker:     "thor1xyz",
		PairID:    "ETH.ETH",
		FromAsset: "ETH.ETH",
		FromE8:    100000000, // 1 ETH
		ToAsset:   "THOR.RUNE",
		ToE8:      500000000, // 5 RUNE
	}

	event, valid := transformSwapEvent(block, e, 1, 0, 1000000000000, 200000000000)
	if !valid {
		t.Fatal("expected valid swap event")
	}
	if event.Asset1In == nil {
		t.Fatal("expected asset1In to be set for Asset->RUNE")
	}
	if event.Asset0Out == nil {
		t.Fatal("expected asset0Out to be set for Asset->RUNE")
	}
	if event.Asset0In != nil {
		t.Fatal("expected asset0In to be nil for Asset->RUNE")
	}
	if event.Asset1Out != nil {
		t.Fatal("expected asset1Out to be nil for Asset->RUNE")
	}
	if event.PriceNative == "" || event.PriceNative == "0" {
		t.Fatalf("expected valid priceNative, got %s", event.PriceNative)
	}
}

func TestTransformSwapEvent_ZeroAmounts_FallsBackToReserves(t *testing.T) {
	block := Block{BlockNumber: 102, BlockTimestamp: 1698126300}
	e := &rawEvent{
		TxnID:     "TXHASH3",
		Maker:     "thor1zero",
		PairID:    "BTC.BTC",
		FromAsset: "THOR.RUNE",
		FromE8:    0,
		ToAsset:   "BTC.BTC",
		ToE8:      0,
	}

	// With valid reserves, should fall back to reserve price.
	event, valid := transformSwapEvent(block, e, 0, 0, 100000000, 50000000)
	if !valid {
		t.Fatal("expected valid event with reserve fallback")
	}
	if event.PriceNative == "" || event.PriceNative == "0" {
		t.Fatalf("expected reserve-based price, got %s", event.PriceNative)
	}
}

func TestTransformSwapEvent_ZeroAmounts_NoReserves_Invalid(t *testing.T) {
	block := Block{BlockNumber: 103, BlockTimestamp: 1698126400}
	e := &rawEvent{
		TxnID:     "TXHASH4",
		Maker:     "thor1none",
		PairID:    "BTC.BTC",
		FromAsset: "THOR.RUNE",
		FromE8:    0,
		ToAsset:   "BTC.BTC",
		ToE8:      0,
	}

	_, valid := transformSwapEvent(block, e, 0, 0, 0, 0)
	if valid {
		t.Fatal("expected invalid event when no price available")
	}
}

func TestTransformSwapEvent_UnexpectedDirection_Invalid(t *testing.T) {
	block := Block{BlockNumber: 104, BlockTimestamp: 1698126500}
	e := &rawEvent{
		TxnID:     "TXHASH5",
		Maker:     "thor1bad",
		PairID:    "BTC.BTC",
		FromAsset: "ETH.ETH",
		FromE8:    100000000,
		ToAsset:   "BTC.BTC",
		ToE8:      5000000,
	}

	_, valid := transformSwapEvent(block, e, 0, 0, 100000000, 50000000)
	if valid {
		t.Fatal("expected invalid for non-RUNE swap direction")
	}
}

func TestTransformLiquidityEvent_Join(t *testing.T) {
	block := Block{BlockNumber: 200, BlockTimestamp: 1698130000}
	e := &rawEvent{
		EventType: "join",
		TxnID:     "TXJOIN1",
		Maker:     "thor1lp",
		PairID:    "BTC.BTC",
		RuneE8:    500000000,
		AssetE8:   2500000,
	}

	event := transformLiquidityEvent(block, e, 0, 0, 1000000000000, 500000000)
	if event.EventType != "join" {
		t.Fatalf("expected join, got %s", event.EventType)
	}
	if event.Amount0 != "5.00000000" {
		t.Fatalf("expected amount0 5.00000000, got %s", event.Amount0)
	}
	if event.Amount1 != "0.02500000" {
		t.Fatalf("expected amount1 0.02500000, got %s", event.Amount1)
	}
}

func TestTransformLiquidityEvent_Exit(t *testing.T) {
	block := Block{BlockNumber: 201, BlockTimestamp: 1698130100}
	e := &rawEvent{
		EventType:   "exit",
		TxnID:       "TXEXIT1",
		Maker:       "thor1lp",
		PairID:      "BTC.BTC",
		EmitRuneE8:  250000000,
		EmitAssetE8: 1250000,
	}

	event := transformLiquidityEvent(block, e, 0, 0, 1000000000000, 500000000)
	if event.EventType != "exit" {
		t.Fatalf("expected exit, got %s", event.EventType)
	}
	if event.Amount0 != "2.50000000" {
		t.Fatalf("expected amount0 2.50000000, got %s", event.Amount0)
	}
	if event.Amount1 != "0.01250000" {
		t.Fatalf("expected amount1 0.01250000, got %s", event.Amount1)
	}
}

func TestMakeReserves(t *testing.T) {
	r := makeReserves(100000000, 50000000)
	if r.Asset0 != "1.00000000" {
		t.Fatalf("expected asset0 1.00000000, got %s", r.Asset0)
	}
	if r.Asset1 != "0.50000000" {
		t.Fatalf("expected asset1 0.50000000, got %s", r.Asset1)
	}
}

func TestParseAssetID(t *testing.T) {
	tests := []struct {
		input          string
		expectedName   string
		expectedSymbol string
	}{
		{"BTC.BTC", "BTC.BTC", "BTC"},
		{"ETH.USDC-0XA0B86991", "ETH.USDC-0XA0B86991", "USDC"},
		{"THOR.RUNE", "THOR.RUNE", "RUNE"},
		{"NODOT", "NODOT", "NODOT"},
	}
	for _, tt := range tests {
		name, symbol := parseAssetID(tt.input)
		if name != tt.expectedName {
			t.Errorf("parseAssetID(%s) name = %s, want %s", tt.input, name, tt.expectedName)
		}
		if symbol != tt.expectedSymbol {
			t.Errorf("parseAssetID(%s) symbol = %s, want %s", tt.input, symbol, tt.expectedSymbol)
		}
	}
}

func TestFindPoolDepth(t *testing.T) {
	lookup := map[string][]poolDepth{
		"BTC.BTC": {
			{Pool: "BTC.BTC", BlockTimestamp: 300, RuneE8: 3000, AssetE8: 30},
			{Pool: "BTC.BTC", BlockTimestamp: 200, RuneE8: 2000, AssetE8: 20},
			{Pool: "BTC.BTC", BlockTimestamp: 100, RuneE8: 1000, AssetE8: 10},
		},
	}

	// Exact match
	rune, asset := findPoolDepth(lookup, "BTC.BTC", 200)
	if rune != 2000 || asset != 20 {
		t.Fatalf("expected (2000,20) at ts=200, got (%d,%d)", rune, asset)
	}

	// Between timestamps — should get the most recent <= 250
	rune, asset = findPoolDepth(lookup, "BTC.BTC", 250)
	if rune != 2000 || asset != 20 {
		t.Fatalf("expected (2000,20) at ts=250, got (%d,%d)", rune, asset)
	}

	// Before all
	rune, asset = findPoolDepth(lookup, "BTC.BTC", 50)
	if rune != 0 || asset != 0 {
		t.Fatalf("expected (0,0) at ts=50, got (%d,%d)", rune, asset)
	}

	// Unknown pool
	rune, asset = findPoolDepth(lookup, "ETH.ETH", 200)
	if rune != 0 || asset != 0 {
		t.Fatalf("expected (0,0) for unknown pool, got (%d,%d)", rune, asset)
	}
}

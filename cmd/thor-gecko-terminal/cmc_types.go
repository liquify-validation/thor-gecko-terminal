// CoinMarketCap (CMC) Integration API - Data types
// Specification: CMC Ideal API Endpoint Samples (Section A: Spot, Section C: DEX)

package main

// CMCSummaryEntry — /summary endpoint response item.
type CMCSummaryEntry struct {
	TradingPairs          string  `json:"trading_pairs"`
	BaseCurrency          string  `json:"base_currency"`
	QuoteCurrency         string  `json:"quote_currency"`
	LastPrice             float64 `json:"last_price"`
	LowestAsk             float64 `json:"lowest_ask"`
	HighestBid            float64 `json:"highest_bid"`
	BaseVolume            float64 `json:"base_volume"`
	QuoteVolume           float64 `json:"quote_volume"`
	PriceChangePercent24h float64 `json:"price_change_percent_24h"`
	HighestPrice24h       float64 `json:"highest_price_24h"`
	LowestPrice24h        float64 `json:"lowest_price_24h"`
}

// CMCAsset — /assets endpoint response item (map value).
type CMCAsset struct {
	Name                 string `json:"name"`
	UnifiedCryptoassetID string `json:"unified_cryptoasset_id,omitempty"`
	CanWithdraw          string `json:"can_withdraw"`
	CanDeposit           string `json:"can_deposit"`
	MinWithdraw          string `json:"min_withdraw"`
	MaxWithdraw          string `json:"max_withdraw"`
	MakerFee             string `json:"maker_fee"`
	TakerFee             string `json:"taker_fee"`
	ContractAddress      string `json:"contractAddress,omitempty"`
	ContractAddressURL   string `json:"contractAddressUrl,omitempty"`
}

// CMCTickerEntry — /ticker endpoint response item (map value).
type CMCTickerEntry struct {
	BaseID      string  `json:"base_id,omitempty"`
	QuoteID     string  `json:"quote_id,omitempty"`
	LastPrice   float64 `json:"last_price"`
	BaseVolume  float64 `json:"base_volume"`
	QuoteVolume float64 `json:"quote_volume"`
	IsFrozen    int     `json:"isFrozen"`
}

// CMCTrade — /trades endpoint response item.
type CMCTrade struct {
	TradeID     string  `json:"trade_id"`
	Price       float64 `json:"price"`
	BaseVolume  float64 `json:"base_volume"`
	QuoteVolume float64 `json:"quote_volume"`
	Timestamp   int64   `json:"timestamp"`
	Type        string  `json:"type"` // "buy" or "sell"
}

// CMCSwap — DEX section C2 (subgraph-style) /swaps endpoint response item.
type CMCSwap struct {
	ID         string      `json:"id"`
	FromAmount string      `json:"fromAmount"`
	ToAmount   string      `json:"toAmount"`
	Timestamp  int64       `json:"timestamp"`
	Pair       CMCSwapPair `json:"pair"`
}

type CMCSwapPair struct {
	FromToken CMCSwapToken `json:"fromToken"`
	ToToken   CMCSwapToken `json:"toToken"`
}

type CMCSwapToken struct {
	Decimals int    `json:"decimals"`
	Symbol   string `json:"symbol"`
}

// GeckoTerminal Integration API - Data types and mappings
// Specification: https://docs.google.com/document/d/1ufjAJUa6rGO9PBGJGwfBMn-XMk9NE0ow3_iMYrS3drk/edit?tab=t.0

package main

// Block represents a THORChain block reference.
type Block struct {
	BlockNumber    int64             `json:"blockNumber"`
	BlockTimestamp int64             `json:"blockTimestamp"`
	Metadata       map[string]string `json:"metadata,omitempty"`
}

type LatestBlockResponse struct {
	Block Block `json:"block"`
}

type Asset struct {
	ID                string  `json:"id"`
	Name              string  `json:"name"`
	Symbol            string  `json:"symbol"`
	Decimals          int     `json:"decimals"`
	TotalSupply       *string `json:"totalSupply,omitempty"`
	CirculatingSupply *string `json:"circulatingSupply,omitempty"`
	CoinGeckoID       *string `json:"coinGeckoId,omitempty"`
}

type AssetResponse struct {
	Asset Asset `json:"asset"`
}

type Pair struct {
	ID                      string  `json:"id"`
	DexKey                  string  `json:"dexKey"`
	Asset0ID                string  `json:"asset0Id"`
	Asset1ID                string  `json:"asset1Id"`
	CreatedAtBlockNumber    *int64  `json:"createdAtBlockNumber,omitempty"`
	CreatedAtBlockTimestamp *int64  `json:"createdAtBlockTimestamp,omitempty"`
	CreatedAtTxnID          *string `json:"createdAtTxnId,omitempty"`
	Creator                 *string `json:"creator,omitempty"`
	FeeBps                  *int    `json:"feeBps,omitempty"`
	Pool                    *Pool   `json:"pool,omitempty"`
}

type Pool struct {
	ID       string            `json:"id"`
	Name     string            `json:"name"`
	AssetIDs []string          `json:"assetIds"`
	PairIDs  []string          `json:"pairIds"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

type Reserves struct {
	Asset0 string `json:"asset0"`
	Asset1 string `json:"asset1"`
}

type PairResponse struct {
	Pair Pair `json:"pair"`
}

type SwapEvent struct {
	Block       Block             `json:"block"`
	EventType   string            `json:"eventType"`
	TxnID       string            `json:"txnId"`
	TxnIndex    int               `json:"txnIndex"`
	EventIndex  int               `json:"eventIndex"`
	Maker       string            `json:"maker"`
	PairID      string            `json:"pairId"`
	Asset0In    *string           `json:"asset0In,omitempty"`
	Asset1In    *string           `json:"asset1In,omitempty"`
	Asset0Out   *string           `json:"asset0Out,omitempty"`
	Asset1Out   *string           `json:"asset1Out,omitempty"`
	PriceNative string            `json:"priceNative"`
	Reserves    Reserves          `json:"reserves"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

type JoinExitEvent struct {
	Block      Block             `json:"block"`
	EventType  string            `json:"eventType"`
	TxnID      string            `json:"txnId"`
	TxnIndex   int               `json:"txnIndex"`
	EventIndex int               `json:"eventIndex"`
	Maker      string            `json:"maker"`
	PairID     string            `json:"pairId"`
	Amount0    string            `json:"amount0"`
	Amount1    string            `json:"amount1"`
	Reserves   Reserves          `json:"reserves"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

type EventsResponse struct {
	Events []interface{} `json:"events"`
}

// rawEvent holds intermediate event data from Midgard DB queries before transformation.
type rawEvent struct {
	EventType      string
	TxnID          string
	Maker          string
	PairID         string
	FromAsset      string
	FromE8         int64
	ToAsset        string
	ToE8           int64
	Asset          string
	AssetE8        int64
	RuneE8         int64
	EmitAssetE8    int64
	EmitRuneE8     int64
	BlockTimestamp int64
	EventID        int64
	BlockNumber    int64
}

// poolDepth holds a pool's reserve snapshot at a specific block.
type poolDepth struct {
	Pool           string
	BlockTimestamp int64
	RuneE8         int64
	AssetE8        int64
}

// coinGeckoMapping maps THORChain asset IDs to CoinGecko token IDs.
var coinGeckoMapping = map[string]string{
	// Native layer 1 assets
	"THOR.RUNE": "thorchain",
	"BTC.BTC":   "bitcoin",
	"ETH.ETH":   "ethereum",
	"AVAX.AVAX": "avalanche-2",
	"BCH.BCH":   "bitcoin-cash",
	"LTC.LTC":   "litecoin",
	"DOGE.DOGE": "dogecoin",
	"GAIA.ATOM": "cosmos",
	"XRP.XRP":   "ripple",
	"TRON.TRX":  "tron",
	"SOL.SOL":   "solana",

	// BSC assets
	"BSC.BNB": "binancecoin",
	"BSC.BTCB-0X7130D2A12B9BCBFAE4F2634D864A1EE1CE3EAD9C": "binance-bitcoin",
	"BSC.BUSD-0XE9E7CEA3DEDCA5984780BAFC599BD69ADD087D56": "binance-peg-busd",
	"BSC.ETH-0X2170ED0880AC9A755FD29B2688956BD959F933F8":  "binance-peg-weth",
	"BSC.TWT-0X4B0F1812E5DF2A09796481FF14017E6005508003":  "trust-wallet-token", // gitleaks:allow
	"BSC.USDC-0X8AC76A51CC950D9822D68B83FE1AD97B32CD580D": "binance-bridged-usdc-bnb-smart-chain",
	"BSC.USDT-0X55D398326F99059FF775485246999027B3197955": "binance-bridged-usdt-bnb-smart-chain",

	// Ethereum assets
	"ETH.AAVE-0X7FC66500C84A76AD7E9C93437BFC5AC33E2DDAE9":  "aave",
	"ETH.DAI-0X6B175474E89094C44DA98B954EEDEAC495271D0F":   "dai",
	"ETH.DPI-0X1494CA1F11D487C2BBE4543E90080AEBA4BA3C2B":   "defipulse-index",
	"ETH.FOX-0XC770EEFAD204B5180DF6A14EE197D99D808EE52D":   "shapeshift-fox-token", // gitleaks:allow
	"ETH.GUSD-0X056FD409E1D7A124BD7017459DFEA2F387B6D5CD":  "gemini-dollar",
	"ETH.LINK-0X514910771AF9CA656AF840DFF83E8264ECF986CA":  "chainlink",
	"ETH.LUSD-0X5F98805A4E8BE255A32880FDEC7F6728C6568BA0":  "liquity-usd",
	"ETH.SNX-0XC011A73EE8576FB46F5E1C5751CA3B9FE0AF2A6F":   "havven",
	"ETH.TGT-0X108A850856DB3F85D0269A2693D896B394C80325":   "thorwallet",
	"ETH.THOR-0XA5F2211B9B8170F694421F2046281775E8468044":  "thorswap",
	"ETH.USDC-0XA0B86991C6218B36C1D19D4A2E9EB0CE3606EB48":  "usd-coin",
	"ETH.USDP-0X8E870D67F660D95D5BE530380D0EC0BD388289E1":  "paxos-standard",
	"ETH.USDT-0XDAC17F958D2EE523A2206206994597C13D831EC7":  "tether",
	"ETH.VTHOR-0X815C23ECA83261B6EC689B60CC4A58B54BC24D8D": "",
	"ETH.WBTC-0X2260FAC5E5542A773AA44FBCFEDF7C193BC2C599":  "wrapped-bitcoin",
	"ETH.XRUNE-0X69FA0FEE221AD11012BAB0FDB45D444D3D2CE71C": "thorstarter",
	"ETH.YFI-0X0BC529C00C6401AEF6D220BE8C6EA1667F6AD93E":   "yearn-finance",

	// Avalanche assets
	"AVAX.SOL-0XFE6B19286885A4F7F55ADAD09C3CD1F906D2478F":  "solana",
	"AVAX.USDC-0XB97EF9EF8734C71904D8002F8B6BC66DD9C48A6E": "usd-coin",
	"AVAX.USDT-0X9702230A8EA53601F5CD2DC00FDBC13D4DF4A8C7": "tether",

	// Base assets
	"BASE.CBBTC-0XCBB7C0000AB88B473B1F5AFD9EF808440EED33BF": "coinbase-wrapped-btc",
	"BASE.ETH": "ethereum",
	"BASE.USDC-0X833589FCD6EDB6E08F4C7C32D4F71B54BDA02913": "usd-coin",
	"BASE.VVV-0XACFE6019ED1A7DC6F7B508C02D1B04EC88CC21BF":  "venice-token",

	// TRON assets
	"TRON.USDT-TR7NHQJEKQXGTCI8Q8ZY4PL8OTSZGJLJ6T": "tether",

	// THORChain native assets (smaller tokens)
	"THOR.RUJI": "rujira",
	"THOR.TCY":  "tcy",
}

// assetNames maps THORChain asset IDs to human-readable display names.
var assetNames = map[string]string{
	"BTC.BTC":   "Bitcoin",
	"ETH.ETH":   "Ethereum",
	"AVAX.AVAX": "Avalanche",
	"BNB.BNB":   "Binance Coin",
	"GAIA.ATOM": "Cosmos",
	"SOL.SOL":   "Solana",
	"DOGE.DOGE": "Dogecoin",
	"LTC.LTC":   "Litecoin",
	"BCH.BCH":   "Bitcoin Cash",
}

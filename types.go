package main

type BlockData struct {
	Block   Block   `json:"block"`
	Network Network `json:"network"`
	Txs     []Tx    `json:"txs"`
}

type Block struct {
	Number    uint64 `json:"number"`
	Timestamp uint64 `json:"timestamp"`
	BaseFee   string `json:"base_fee"`
	GasUsed   uint64 `json:"gas_used"`
}

type Network struct {
	Mempool APIMempoolStats `json:"mempool"`
	History BlockHistory    `json:"history"`
}

type BlockHistory struct {
	GasRatio5   float64 `json:"gas_ratio_5"`
	GasSpikes25 int     `json:"gas_spikes_25"`
	FeeEWMA10   float64 `json:"fee_ewma_10"`
	FeeEWMA25   float64 `json:"fee_ewma_25"`
}

type Tx struct {
	Hash            string `json:"hash"`
	SubmissionBlock uint64 `json:"submission_block"`
	MaxFee          string `json:"max_fee"`          // Wei as string
	MaxPriorityFee  string `json:"max_priority_fee"` // Wei as string
	BaseFee         string `json:"base_fee"`         // Wei as string
	Gas             uint64 `json:"gas"`
	InclusionBlocks uint64 `json:"inclusion_blocks"`
}

type NetworkData struct {
	Mempool MempoolData  `json:"mempool"`
	Block   BlockData    `json:"block"`
	History BlockHistory `json:"history"`
}

// WebSocket specific response
type WSResponse struct {
	// Mempool stats with histogram
	Count        uint64         `json:"count"`
	FeeHistogram map[string]int `json:"fee_histogram"`
	BaseFeeTrend float64        `json:"base_fee_trend"`

	NextBaseFee float64 `json:"next_base_fee"`

	CurrentBlockNumber uint64 `json:"current_block"`

	LastBlockTime int64 `json:"last_block_time"`

	// Block history data
	GasRatio5   float64 `json:"gas_ratio_5"`
	GasSpikes25 int     `json:"gas_spikes_25"`
	FeeEWMA10   float64 `json:"fee_ewma_10"`
	FeeEWMA25   float64 `json:"fee_ewma_25"`
}

// API specific response
type APIResponse struct {
	// Mempool percentile stats
	Count uint64  `json:"count"`
	P10   float64 `json:"p10"`
	P30   float64 `json:"p30"`
	P50   float64 `json:"p50"`
	P70   float64 `json:"p70"`
	P90   float64 `json:"p90"`

	NextBlock   uint64  `json:"next_block"`
	NextBaseFee float64 `json:"next_base_fee"`

	History BlockHistory `json:"history"`
}

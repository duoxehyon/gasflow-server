package main

import (
	"fmt"
	"math"
	"math/big"
	"sort"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

const maxMempoolSize = 10000

// Core mempool data structures
type MempoolTx struct {
	Hash           common.Hash
	MaxFee         float64
	MaxPriorityFee float64
	Gas            uint64
	FirstSeen      uint64
	Timestamp      time.Time
}

// Shared data structure that holds all mempool data
type MempoolData struct {
	Count uint64
	// Mempool base fee and block time
	CurrentBaseFee     float64
	CurrentBlockNumber uint64

	LastBlockTime int64
	LastBaseFee   float64

	BaseFeeTrend float64
	Transactions map[common.Hash]*MempoolTx
}

// WebSocket response format
type WSMempoolStats struct {
	Count        uint64
	FeeHistogram map[string]int

	NextBaseFee float64
	NextBlock   uint64

	LastBlockTime int64

	BaseFeeTrend float64
}

// API response format
type APIMempoolStats struct {
	Count       uint64
	P10         float64
	P30         float64
	P50         float64
	P70         float64
	P90         float64
	NextBlock   uint64
	NextBaseFee float64
}

type Mempool struct {
	data MempoolData
	mu   sync.RWMutex
	ttl  time.Duration

	// Callbacks
	onWSUpdate  func(WSMempoolStats)
	onAPIUpdate func(APIMempoolStats)
}

func NewMempool() *Mempool {
	m := &Mempool{
		data: MempoolData{
			Transactions: make(map[common.Hash]*MempoolTx),
		},
		ttl: time.Minute,
	}
	go m.startCleanupRoutine()
	return m
}

func (m *Mempool) Add(tx *types.Transaction, blockNum uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.data.Transactions) >= maxMempoolSize {
		m.evictOldestTx()
	}

	maxFee := weiToGwei(tx.GasFeeCap())
	maxPriorityFee := weiToGwei(tx.GasTipCap())

	m.data.Transactions[tx.Hash()] = &MempoolTx{
		Hash:           tx.Hash(),
		MaxFee:         maxFee,
		MaxPriorityFee: maxPriorityFee,
		Gas:            tx.Gas(),
		FirstSeen:      uint64(m.data.LastBlockTime),
		Timestamp:      time.Now(),
	}
	m.data.Count = uint64(len(m.data.Transactions))

	m.notifyUpdates()
}

func (m *Mempool) evictOldestTx() {
	var oldestTxHash common.Hash
	var oldestTimestamp time.Time

	for hash, tx := range m.data.Transactions {
		if oldestTimestamp.IsZero() || tx.Timestamp.Before(oldestTimestamp) {
			oldestTimestamp = tx.Timestamp
			oldestTxHash = hash
		}
	}

	// Remove the oldest transaction
	if oldestTxHash != (common.Hash{}) {
		delete(m.data.Transactions, oldestTxHash)
	}
}

func calculateNextBaseFee(currentBaseFee float64, gasUsed, gasTarget uint64) float64 {
	if gasUsed == 0 {
		return currentBaseFee * 0.875 // Decrease by 12.5%
	}
	gasUsedFloat := float64(gasUsed)
	gasTargetFloat := float64(gasTarget)
	if gasUsedFloat == gasTargetFloat {
		return currentBaseFee
	}
	delta := (gasUsedFloat - gasTargetFloat) / gasTargetFloat
	changeFactor := 1.0 + math.Max(math.Min(delta/8.0, 0.125), -0.125)
	return currentBaseFee * changeFactor
}

func (m *Mempool) UpdateBlockData(baseFee *big.Int, blockNum uint64, gasUsed, gasLimit uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.data.CurrentBaseFee = calculateNextBaseFee(weiToGwei(baseFee), gasUsed, gasLimit/2)
	m.data.CurrentBlockNumber = blockNum + 1
	m.data.LastBlockTime = time.Now().Unix()
	m.data.LastBaseFee = weiToGwei(baseFee)

	if m.data.LastBaseFee > 0 {
		m.data.BaseFeeTrend = ((m.data.CurrentBaseFee - m.data.LastBaseFee) / m.data.LastBaseFee) * 100
	}
}

func (m *Mempool) RemoveConfirmed(txHashes []common.Hash) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, hash := range txHashes {
		delete(m.data.Transactions, hash)
	}
	m.data.Count = uint64(len(m.data.Transactions))

	m.notifyUpdates()
}

func (m *Mempool) GetTxByHash(hash common.Hash) (*MempoolTx, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	tx, exists := m.data.Transactions[hash]
	return tx, exists
}

// GetWSStats returns mempool statistics for WebSocket clients
func (m *Mempool) GetWSStats() WSMempoolStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	histogram := make(map[string]int)
	for _, tx := range m.data.Transactions {
		bucketStr := fmt.Sprintf("%.1f", tx.MaxPriorityFee)
		histogram[bucketStr]++
	}

	return WSMempoolStats{
		Count:         uint64(len(m.data.Transactions)),
		FeeHistogram:  histogram,
		BaseFeeTrend:  m.data.BaseFeeTrend,
		NextBlock:     m.data.CurrentBlockNumber,
		NextBaseFee:   m.data.CurrentBaseFee,
		LastBlockTime: m.data.LastBlockTime,
	}
}

// GetAPIStats returns mempool statistics for API clients
func (m *Mempool) GetAPIStats() APIMempoolStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	fees := make([]float64, 0, len(m.data.Transactions))
	for _, tx := range m.data.Transactions {
		fees = append(fees, tx.MaxPriorityFee)
	}
	sort.Float64s(fees)

	return APIMempoolStats{
		Count:       m.data.Count,
		P10:         getPercentile(fees, 10),
		P30:         getPercentile(fees, 30),
		P50:         getPercentile(fees, 50),
		P70:         getPercentile(fees, 70),
		P90:         getPercentile(fees, 90),
		NextBlock:   m.data.CurrentBlockNumber,
		NextBaseFee: m.data.CurrentBaseFee, // Next == Current in mempool
	}
}

func (m *Mempool) SetWSCallback(callback func(WSMempoolStats)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onWSUpdate = callback
}

func (m *Mempool) SetAPICallback(callback func(APIMempoolStats)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onAPIUpdate = callback
}

func (m *Mempool) notifyUpdates() {
	if m.onWSUpdate != nil {
		m.onWSUpdate(m.GetWSStats())
	}
	if m.onAPIUpdate != nil {
		m.onAPIUpdate(m.GetAPIStats())
	}
}

func (m *Mempool) startCleanupRoutine() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		m.mu.Lock()
		cutoff := time.Now().Add(-m.ttl)
		for hash, tx := range m.data.Transactions {
			if tx.Timestamp.Before(cutoff) {
				delete(m.data.Transactions, hash)
			}
		}
		m.data.Count = uint64(len(m.data.Transactions))
		m.mu.Unlock()

		m.notifyUpdates()
	}
}

// Helper functions (getPercentile and weiToGwei remain the same)
func getPercentile(values []float64, percentile float64) float64 {
	if len(values) == 0 {
		return 0
	}

	idx := int(float64(len(values)) * percentile / 100)
	if idx >= len(values) {
		idx = len(values) - 1
	}
	return values[idx]
}

func weiToGwei(wei *big.Int) float64 {
	if wei == nil {
		return 0
	}
	gwei := new(big.Float).Quo(new(big.Float).SetInt(wei), new(big.Float).SetInt64(1e9))
	result, _ := gwei.Float64()
	return result
}

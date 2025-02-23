package main

import (
	"fmt"
	"log"
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

type DataManager struct {
	mempool    *Mempool
	blockchain *Blockchain
	storage    *Storage
	ready      bool
	mu         sync.RWMutex

	// Callbacks
	onWSUpdate func(WSResponse)
}

func NewDataManager(endpoint string, outputDir string) (*DataManager, error) {
	blockchain, err := NewBlockchain(endpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to create blockchain: %v", err)
	}

	mempool := NewMempool()

	var storage *Storage
	if outputDir != "" { // Only create storage if outputDir is provided
		storage, err = NewStorage(outputDir)
		if err != nil {
			return nil, fmt.Errorf("failed to create storage: %v", err)
		}
	}

	manager := &DataManager{
		mempool:    mempool,
		blockchain: blockchain,
		storage:    storage,
	}

	// Set up blockchain handlers
	blockchain.SetTxHandler(manager.handleNewTx)
	blockchain.SetBlockHandler(manager.handleNewBlock)
	blockchain.SetBaseFeeHandler(manager.handleBaseFee)

	return manager, nil
}

func (m *DataManager) handleNewBlock(txHashes []common.Hash) {
	m.mu.RLock()
	ready := m.ready
	m.mu.RUnlock()

	if !ready {
		logger.Warn("Skipping block save - warmup not complete")
		return
	}

	// Get block data first
	currentBlock := m.blockchain.GetCurrentBlock()
	blockHistory, err := m.blockchain.GetBlockHistory()
	if err != nil {
		log.Printf("Failed to get block history: %v", err)
		return
	}

	// Collect transaction data before removing from mempool
	txs := make([]Tx, 0, len(txHashes))
	for _, hash := range txHashes {
		if mempoolTx, exists := m.mempool.GetTxByHash(hash); exists {
			tx := Tx{
				Hash:            hash.Hex(),
				SubmissionBlock: mempoolTx.FirstSeen,
				MaxFee:          weiToString(mempoolTx.MaxFee),
				MaxPriorityFee:  weiToString(mempoolTx.MaxPriorityFee),
				BaseFee:         currentBlock.Block.BaseFee,
				Gas:             mempoolTx.Gas,
				InclusionBlocks: currentBlock.Block.Number - mempoolTx.FirstSeen,
			}
			txs = append(txs, tx)
		}
	}

	// Remove from mempool
	m.mempool.RemoveConfirmed(txHashes)

	// Save complete block data
	if m.storage != nil {
		fullBlockData := BlockData{
			Block: currentBlock.Block,
			Network: Network{
				Mempool: m.mempool.GetAPIStats(),
				History: *blockHistory,
			},
			Txs: txs,
		}
		m.storage.SaveBlock(&fullBlockData)
	}

	m.notifyUpdates()
}

func (m *DataManager) handleNewTx(tx *types.Transaction, sender common.Address) {
	currentBlock := m.blockchain.GetCurrentBlock()
	m.mempool.Add(tx, currentBlock.Block.Number)
	m.notifyUpdates()
}

func (m *DataManager) handleBaseFee(baseFee *big.Int, blockNum uint64) {
	currentBlock := m.blockchain.GetCurrentBlock()
	m.mempool.UpdateBlockData(baseFee, blockNum, currentBlock.Block.GasUsed, 35950000) // Default gas limit
	m.notifyUpdates()
}

func (m *DataManager) notifyUpdates() {
	if m.onWSUpdate == nil {
		return
	}

	// Get current data
	mempoolStats := m.mempool.GetWSStats()
	blockHistory, err := m.blockchain.GetBlockHistory()
	if err != nil {
		log.Printf("Failed to get block history: %v", err)
		return
	}

	// Combine data for WebSocket response
	response := WSResponse{
		Count:              mempoolStats.Count,
		FeeHistogram:       mempoolStats.FeeHistogram,
		BaseFeeTrend:       mempoolStats.BaseFeeTrend,
		NextBaseFee:        mempoolStats.NextBaseFee,
		LastBlockTime:      mempoolStats.LastBlockTime,
		CurrentBlockNumber: mempoolStats.NextBlock - 1,
		GasRatio5:          blockHistory.GasRatio5,
		GasSpikes25:        blockHistory.GasSpikes25,
		FeeEWMA10:          blockHistory.FeeEWMA10,
		FeeEWMA25:          blockHistory.FeeEWMA25,
	}

	m.onWSUpdate(response)
}

func (m *DataManager) GetAPIResponse() (*APIResponse, error) {
	mempoolStats := m.mempool.GetAPIStats()
	blockHistory, err := m.blockchain.GetBlockHistory()
	if err != nil {
		return nil, err
	}

	return &APIResponse{
		Count:       mempoolStats.Count,
		P10:         mempoolStats.P10,
		P30:         mempoolStats.P30,
		P50:         mempoolStats.P50,
		P70:         mempoolStats.P70,
		P90:         mempoolStats.P90,
		NextBlock:   mempoolStats.NextBlock,
		NextBaseFee: mempoolStats.NextBaseFee,
		History:     *blockHistory,
	}, nil
}

func (m *DataManager) SetWSCallback(callback func(WSResponse)) {
	m.onWSUpdate = callback
}

func (m *DataManager) Start() error {
	if m.storage != nil {
		m.storage.Start()
	}

	// Start blockchain first
	if err := m.blockchain.Start(); err != nil {
		return err
	}

	// Warmup in background to not block startup
	go func() {
		logger.Info("Warming up mempool for 10 seconds...")
		time.Sleep(10 * time.Second)

		m.mu.Lock()
		m.ready = true
		m.mu.Unlock()

		logger.Info("✅ Warmup complete")
	}()

	return nil
}

func (m *DataManager) Stop() {
	m.blockchain.Stop()
	if m.storage != nil {
		m.storage.Stop()
	}
}

func weiToString(gweiValue float64) string {
	// Convert Gwei back to Wei
	weiFloat := gweiValue * 1e9
	return fmt.Sprintf("%.0f", weiFloat)
}

package main

import (
	"context"
	"fmt"
	"log"
	"math/big"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
)

type TxHandler func(*types.Transaction, common.Address)
type BlockHandler func([]common.Hash)
type BaseFeeHandler func(*big.Int, uint64)

type Blockchain struct {
	client    *ethclient.Client
	rpcClient *rpc.Client
	ctx       context.Context
	cancel    context.CancelFunc

	// Block window
	blockWindow struct {
		blocks      [25]*types.Block
		lastIndex   int
		initialized bool
		mu          sync.RWMutex
	}

	// Current block data
	currentBlock struct {
		data BlockData
		mu   sync.RWMutex
	}

	// Handlers
	handleTx      TxHandler
	handleBlock   BlockHandler
	handleBaseFee BaseFeeHandler
}

func NewBlockchain(endpoint string) (*Blockchain, error) {
	ctx, cancel := context.WithCancel(context.Background())

	client, err := ethclient.Dial(endpoint)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to connect to Ethereum client: %v", err)
	}

	rpcClient, err := rpc.Dial(endpoint)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to connect to RPC client: %v", err)
	}

	return &Blockchain{
		client:    client,
		rpcClient: rpcClient,
		ctx:       ctx,
		cancel:    cancel,
	}, nil
}

func (b *Blockchain) updateBlockWindow(newBlock *types.Block) error {
	b.blockWindow.mu.Lock()
	defer b.blockWindow.mu.Unlock()

	if !b.blockWindow.initialized {
		for i := 24; i >= 0; i-- {
			blockNum := newBlock.NumberU64() - uint64(i) - 1
			block, err := b.client.BlockByNumber(b.ctx, big.NewInt(int64(blockNum)))
			if err != nil {
				return fmt.Errorf("failed to get historical block %d: %v", blockNum, err)
			}
			b.blockWindow.blocks[24-i] = block
		}
		b.blockWindow.initialized = true
		b.blockWindow.lastIndex = 24
		return nil
	}

	b.blockWindow.lastIndex = (b.blockWindow.lastIndex + 1) % 25
	b.blockWindow.blocks[b.blockWindow.lastIndex] = newBlock
	return nil
}

func (b *Blockchain) GetBlockHistory() (*BlockHistory, error) {
	b.blockWindow.mu.RLock()
	defer b.blockWindow.mu.RUnlock()

	if !b.blockWindow.initialized {
		return nil, fmt.Errorf("block window not initialized")
	}

	var gasRatios []float64
	var gasSpikes int
	var fees []float64

	blocks := make([]*types.Block, 25)
	for i := 0; i < 25; i++ {
		idx := (b.blockWindow.lastIndex - i + 25) % 25
		blocks[i] = b.blockWindow.blocks[idx]
	}

	for _, block := range blocks {
		gasRatio := float64(block.GasUsed()) / float64(block.GasLimit())
		gasRatios = append(gasRatios, gasRatio)

		if gasRatio > 0.9 {
			gasSpikes++
		}

		fees = append(fees, weiToGwei(block.BaseFee()))
	}

	return &BlockHistory{
		GasRatio5:   average(gasRatios[:5]),
		GasSpikes25: gasSpikes,
		FeeEWMA10:   calculateEWMA(fees[:10], 0.2),
		FeeEWMA25:   calculateEWMA(fees, 0.1),
	}, nil
}

func (b *Blockchain) updateCurrentBlock(block *types.Block) {
	b.currentBlock.mu.Lock()
	defer b.currentBlock.mu.Unlock()

	b.currentBlock.data = BlockData{
		Block: Block{
			Number:    block.NumberU64(),
			Timestamp: block.Time(),
			BaseFee:   block.BaseFee().String(),
			GasUsed:   block.GasUsed(),
		},
		Network: Network{
			Mempool: APIMempoolStats{},
			History: BlockHistory{},
		},
		Txs: []Tx{},
	}
}

func (b *Blockchain) GetCurrentBlock() BlockData {
	b.currentBlock.mu.RLock()
	defer b.currentBlock.mu.RUnlock()
	return b.currentBlock.data
}

func (b *Blockchain) SetTxHandler(handler TxHandler) {
	b.handleTx = handler
}

func (b *Blockchain) SetBlockHandler(handler BlockHandler) {
	b.handleBlock = handler
}

func (b *Blockchain) SetBaseFeeHandler(handler BaseFeeHandler) {
	b.handleBaseFee = handler
}

func (b *Blockchain) Start() error {
	header, err := b.client.HeaderByNumber(context.Background(), nil)
	if err != nil {
		return fmt.Errorf("failed to get latest block: %v", err)
	}

	block, err := b.client.BlockByHash(context.Background(), header.Hash())
	if err != nil {
		return fmt.Errorf("failed to get latest block: %v", err)
	}

	if err := b.updateBlockWindow(block); err != nil {
		return fmt.Errorf("failed to initialize block window: %v", err)
	}
	b.updateCurrentBlock(block)

	logger.Info("Block window initialized")

	pendingTxs := make(chan common.Hash)
	pendingSub, err := b.rpcClient.EthSubscribe(context.Background(), pendingTxs, "newPendingTransactions")
	if err != nil {
		return fmt.Errorf("failed to subscribe to pending transactions: %v", err)
	}

	headers := make(chan *types.Header)
	headerSub, err := b.client.SubscribeNewHead(b.ctx, headers)
	if err != nil {
		pendingSub.Unsubscribe()
		return fmt.Errorf("failed to subscribe to new headers: %v", err)
	}

	logger.Info("Connected to Ethereum network")

	go func() {
		defer pendingSub.Unsubscribe()
		defer headerSub.Unsubscribe()

		for {
			select {
			case <-b.ctx.Done():
				return

			case err := <-pendingSub.Err():
				log.Printf("Pending tx subscription error: %v", err)
				return

			case err := <-headerSub.Err():
				log.Printf("Header subscription error: %v", err)
				return

			case txHash := <-pendingTxs:
				if b.handleTx == nil {
					continue
				}

				tx, isPending, err := b.client.TransactionByHash(context.Background(), txHash)
				if err != nil || !isPending {
					continue
				}

				sender, err := types.Sender(types.NewCancunSigner(tx.ChainId()), tx)
				if err != nil {
					continue
				}

				b.handleTx(tx, sender)

			case header := <-headers:
				block, err := b.client.BlockByHash(b.ctx, header.Hash())
				if err != nil {
					continue
				}

				if err := b.updateBlockWindow(block); err != nil {
					log.Printf("Failed to update block window: %v", err)
				}
				b.updateCurrentBlock(block)

				// Handle base fee updates
				if b.handleBaseFee != nil {
					b.handleBaseFee(header.BaseFee, header.Number.Uint64())
				}

				// Handle confirmed transactions
				if b.handleBlock != nil {
					var txHashes []common.Hash
					for _, tx := range block.Transactions() {
						txHashes = append(txHashes, tx.Hash())
					}
					b.handleBlock(txHashes)
				}
			}
		}
	}()

	return nil
}

func (b *Blockchain) Stop() {
	b.cancel()
}

// Helper functions
func average(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

func calculateEWMA(values []float64, alpha float64) float64 {
	if len(values) == 0 {
		return 0
	}

	ewma := values[0]
	for i := 1; i < len(values); i++ {
		ewma = alpha*values[i] + (1-alpha)*ewma
	}
	return ewma
}

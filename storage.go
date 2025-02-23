package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/sirupsen/logrus"
)

type Storage struct {
	outputDir string
	saveQueue chan *BlockData
	wg        sync.WaitGroup
	shutdown  chan struct{}
}

func NewStorage(outputDir string) (*Storage, error) {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %v", err)
	}

	return &Storage{
		outputDir: outputDir,
		saveQueue: make(chan *BlockData, 100), // Buffer 100 blocks
		shutdown:  make(chan struct{}),
	}, nil
}

func (s *Storage) Start() {
	s.wg.Add(1)
	go s.processQueue()
}

func (s *Storage) Stop() {
	close(s.shutdown)
	s.wg.Wait()
}

func (s *Storage) SaveBlock(data *BlockData) {
	select {
	case s.saveQueue <- data:
	default:
		log.Printf("Warning: Block save queue is full, dropping block %d", data.Block.Number)
	}
}

func (s *Storage) processQueue() {
	defer s.wg.Done()

	for {
		select {
		case <-s.shutdown:
			// Process remaining items in queue before shutting down
			for len(s.saveQueue) > 0 {
				data := <-s.saveQueue
				s.saveBlockToFile(data)
			}
			return

		case data := <-s.saveQueue:
			s.saveBlockToFile(data)
		}
	}
}

func (s *Storage) saveBlockToFile(data *BlockData) {
	filename := filepath.Join(s.outputDir, fmt.Sprintf("block_%d.json", data.Block.Number))

	if _, err := os.Stat(filename); err == nil {
		log.Printf("Block %d already saved, skipping", data.Block.Number)
		return
	}

	file, err := os.Create(filename)
	if err != nil {
		log.Printf("Failed to create file for block %d: %v", data.Block.Number, err)
		return
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "    ")
	if err := encoder.Encode(data); err != nil {
		log.Printf("Failed to save block %d: %v", data.Block.Number, err)
		// If we failed to write, try to clean up the file
		file.Close()
		os.Remove(filename)
		return
	}

	logger.WithFields(logrus.Fields{
		"block_number": data.Block.Number,
		"tx_count":     len(data.Txs),
	}).Info("Block data saved")
}

// GetBlock retrieves a specific block from storage
func (s *Storage) GetBlock(blockNum uint64) (*BlockData, error) {
	// Try the standard filename first
	filename := filepath.Join(s.outputDir, fmt.Sprintf("block_%d.json", blockNum))

	// If not found, try to find a timestamped version
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		pattern := filepath.Join(s.outputDir, fmt.Sprintf("block_%d_*.json", blockNum))
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return nil, err
		}
		if len(matches) == 0 {
			return nil, fmt.Errorf("block %d not found", blockNum)
		}
		// Use the most recent version if multiple exist
		sort.Strings(matches)
		filename = matches[len(matches)-1]
	}

	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var blockData BlockData
	if err := json.NewDecoder(file).Decode(&blockData); err != nil {
		return nil, err
	}

	return &blockData, nil
}

// ListBlocks returns a sorted list of all block numbers in storage
func (s *Storage) ListBlocks() ([]uint64, error) {
	pattern := filepath.Join(s.outputDir, "block_*.json")
	files, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}

	blockMap := make(map[uint64]struct{})
	for _, file := range files {
		base := filepath.Base(file)
		var blockNum uint64
		// Try both formats: block_X.json and block_X_timestamp.json
		_, err := fmt.Sscanf(base, "block_%d.json", &blockNum)
		if err != nil {
			_, err = fmt.Sscanf(base, "block_%d_", &blockNum)
			if err != nil {
				continue
			}
		}
		blockMap[blockNum] = struct{}{}
	}

	blocks := make([]uint64, 0, len(blockMap))
	for blockNum := range blockMap {
		blocks = append(blocks, blockNum)
	}
	sort.Slice(blocks, func(i, j int) bool {
		return blocks[i] < blocks[j]
	})

	return blocks, nil
}

// GetLatestBlock retrieves the most recent block from storage
func (s *Storage) GetLatestBlock() (*BlockData, error) {
	blocks, err := s.ListBlocks()
	if err != nil {
		return nil, err
	}
	if len(blocks) == 0 {
		return nil, fmt.Errorf("no blocks in storage")
	}
	return s.GetBlock(blocks[len(blocks)-1])
}

// GetBlockRange retrieves a range of blocks [start, end]
func (s *Storage) GetBlockRange(start, end uint64) ([]*BlockData, error) {
	if start > end {
		return nil, fmt.Errorf("invalid range: start > end")
	}

	var blocks []*BlockData
	for i := start; i <= end; i++ {
		block, err := s.GetBlock(i)
		if err != nil {
			// Skip blocks that don't exist
			continue
		}
		blocks = append(blocks, block)
	}

	if len(blocks) == 0 {
		return nil, fmt.Errorf("no blocks found in range %d-%d", start, end)
	}

	return blocks, nil
}

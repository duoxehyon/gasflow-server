package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
)

type APIServer struct {
	port    string
	manager *DataManager
}

func NewAPIServer(port string, manager *DataManager) *APIServer {
	return &APIServer{
		port:    port,
		manager: manager,
	}
}

func (s *APIServer) Start() error {
	// Internal endpoints
	http.HandleFunc("/internal/network", s.handleInternalRequest(s.handleNetworkData))
	http.HandleFunc("/internal/block", s.handleInternalRequest(s.handleBlockData))

	log.Printf("🔒 Internal API server starting on %s", s.port)
	go func() {
		if err := http.ListenAndServe(s.port, nil); err != nil {
			log.Printf("API server error: %v", err)
		}
	}()

	return nil
}

func (s *APIServer) handleInternalRequest(handler func(http.ResponseWriter, *http.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Only allow GET requests
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		handler(w, r)
	}
}

func (s *APIServer) handleNetworkData(w http.ResponseWriter, r *http.Request) {
	response, err := s.manager.GetAPIResponse()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get network data: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (s *APIServer) handleBlockData(w http.ResponseWriter, r *http.Request) {
	blockNum := r.URL.Query().Get("block")
	if blockNum == "" {
		http.Error(w, "Block number required", http.StatusBadRequest)
		return
	}

	num, err := strconv.ParseUint(blockNum, 10, 64)
	if err != nil {
		http.Error(w, "Invalid block number", http.StatusBadRequest)
		return
	}

	// Use manager to get block data
	block := s.manager.blockchain.GetCurrentBlock()
	if block.Block.Number != num {
		http.Error(w, "Block not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(block)
}

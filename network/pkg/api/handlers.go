package api

import (
	"net/http"
	"strconv"

	"github.com/aporia-zero/network/pkg/types"
	"github.com/gin-gonic/gin"
)

// Node handlers
func (s *APIServer) handleGetNodeInfo(c *gin.Context) {
	info := s.services.NodeService.GetNodeInfo()
	successResponse(c, info)
}

func (s *APIServer) handleGetPeers(c *gin.Context) {
	peers := s.services.NodeService.GetPeers()
	successResponse(c, peers)
}

func (s *APIServer) handleAddPeer(c *gin.Context) {
	var request struct {
		Address string `json:"address" binding:"required"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		errorResponse(c, http.StatusBadRequest, err)
		return
	}

	if err := s.services.NodeService.AddPeer(request.Address); err != nil {
		errorResponse(c, http.StatusInternalServerError, err)
		return
	}

	successResponse(c, gin.H{"message": "Peer added successfully"})
}

func (s *APIServer) handleRemovePeer(c *gin.Context) {
	id := c.Param("id")
	if err := s.services.NodeService.RemovePeer(id); err != nil {
		errorResponse(c, http.StatusInternalServerError, err)
		return
	}

	successResponse(c, gin.H{"message": "Peer removed successfully"})
}

// Transaction handlers
func (s *APIServer) handleSubmitTransaction(c *gin.Context) {
	var tx types.Transaction
	if err := c.ShouldBindJSON(&tx); err != nil {
		errorResponse(c, http.StatusBadRequest, err)
		return
	}

	if err := s.services.TxService.SubmitTransaction(&tx); err != nil {
		errorResponse(c, http.StatusInternalServerError, err)
		return
	}

	successResponse(c, gin.H{
		"message": "Transaction submitted successfully",
		"hash":    tx.Hash,
	})
}

func (s *APIServer) handleGetTransaction(c *gin.Context) {
	hash := c.Param("hash")
	tx, err := s.services.TxService.GetTransaction(hash)
	if err != nil {
		errorResponse(c, http.StatusNotFound, err)
		return
	}

	successResponse(c, tx)
}

func (s *APIServer) handleGetPendingTransactions(c *gin.Context) {
	txs, err := s.services.TxService.GetPendingTransactions()
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, err)
		return
	}

	successResponse(c, txs)
}

func (s *APIServer) handleGetTransactionStatus(c *gin.Context) {
	hash := c.Param("hash")
	status, err := s.services.TxService.GetTransactionStatus(hash)
	if err != nil {
		errorResponse(c, http.StatusNotFound, err)
		return
	}

	successResponse(c, gin.H{"status": status})
}

// Block handlers
func (s *APIServer) handleGetBlock(c *gin.Context) {
	hash := c.Param("hash")
	block, err := s.services.BlockService.GetBlock(hash)
	if err != nil {
		errorResponse(c, http.StatusNotFound, err)
		return
	}

	successResponse(c, block)
}

func (s *APIServer) handleGetBlockByHeight(c *gin.Context) {
	heightStr := c.Param("height")
	height, err := strconv.ParseUint(heightStr, 10, 64)
	if err != nil {
		errorResponse(c, http.StatusBadRequest, err)
		return
	}

	block, err := s.services.BlockService.GetBlockByHeight(height)
	if err != nil {
		errorResponse(c, http.StatusNotFound, err)
		return
	}

	successResponse(c, block)
}

func (s *APIServer) handleGetBlockRange(c *gin.Context) {
	startStr := c.Query("start")
	endStr := c.Query("end")

	start, err := strconv.ParseUint(startStr, 10, 64)
	if err != nil {
		errorResponse(c, http.StatusBadRequest, err)
		return
	}

	end, err := strconv.ParseUint(endStr, 10, 64)
	if err != nil {
		errorResponse(c, http.StatusBadRequest, err)
		return
	}

	blocks, err := s.services.BlockService.GetBlockRange(start, end)
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, err)
		return
	}

	successResponse(c, blocks)
}

func (s *APIServer) handleGetLatestBlock(c *gin.Context) {
	block, err := s.services.BlockService.GetLatestBlock()
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, err)
		return
	}

	successResponse(c, block)
}

// State handlers
func (s *APIServer) handleGetAccount(c *gin.Context) {
	id := c.Param("id")
	account, err := s.services.StateService.GetAccount(id)
	if err != nil {
		errorResponse(c, http.StatusNotFound, err)
		return
	}

	successResponse(c, account)
}

func (s *APIServer) handleGetStateRoot(c *gin.Context) {
	root, err := s.services.StateService.GetStateRoot()
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, err)
		return
	}

	successResponse(c, gin.H{"root": root})
}

func (s *APIServer) handleGetStateProof(c *gin.Context) {
	account := c.Param("account")
	proof, err := s.services.StateService.GetStateProof(account)
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, err)
		return
	}

	successResponse(c, gin.H{"proof": proof})
}

// Metrics handler
func (s *APIServer) handleMetrics(c *gin.Context) {
	// Implement metrics collection and reporting
	metrics := collectMetrics()
	successResponse(c, metrics)
}

// Swagger handler
func (s *APIServer) handleSwagger(c *gin.Context) {
	// Serve swagger documentation
	c.File("./swagger/index.html")
}

// Helper function to collect metrics
func collectMetrics() map[string]interface{} {
	return map[string]interface{}{
		"uptime":           uptimeMetrics(),
		"memory":           memoryMetrics(),
		"goroutines":       goroutineMetrics(),
		"transaction_pool": transactionPoolMetrics(),
		"network":          networkMetrics(),
	}
}

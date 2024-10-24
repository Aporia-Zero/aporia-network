use super::types::{Block, ConsensusConfig, ConsensusState, ValidatorId};
use super::errors::ConsensusError;
use ark_ec::PairingEngine;
use ark_ff::Field;
use std::sync::Arc;
use tokio::sync::RwLock;
use sha3::{Sha3_256, Digest};
use std::time::{SystemTime, UNIX_EPOCH};

/// Block production management
pub struct BlockProducer<E: PairingEngine> {
    /// Consensus configuration
    config: ConsensusConfig,
    
    /// Current consensus state
    state: Arc<RwLock<ConsensusState<E>>>,
    
    /// Last produced block time
    last_block_time: Arc<RwLock<u64>>,
}

impl<E: PairingEngine> BlockProducer<E> {
    /// Create new block producer
    pub fn new(
        config: ConsensusConfig,
        state: Arc<RwLock<ConsensusState<E>>>,
    ) -> Self {
        Self {
            config,
            state,
            last_block_time: Arc::new(RwLock::new(0)),
        }
    }

    /// Start block producer
    pub async fn start(&self) -> Result<(), ConsensusError> {
        // Initialize block production parameters
        *self.last_block_time.write().await = self.current_time()?;
        Ok(())
    }

    /// Create new block
    pub async fn create_block(
        &self,
        producer: ValidatorId,
        identity_proof: Vec<u8>,
    ) -> Result<Block<E>, ConsensusError> {
        let state = self.state.read().await;
        let current_time = self.current_time()?;
        
        // Check block time
        self.verify_block_time(current_time).await?;
        
        // Create block
        let block = Block {
            height: state.height + 1,
            timestamp: current_time,
            prev_hash: state.last_block_hash,
            hash: self.calculate_block_hash(&state)?,
            producer,
            identity_proof: identity_proof.into(),
            epoch_length: self.config.epoch_length,
        };
        
        // Update last block time
        *self.last_block_time.write().await = current_time;
        
        Ok(block)
    }

    /// Verify block
    pub async fn verify_block(&self, block: &Block<E>) -> Result<(), ConsensusError> {
        // Verify block structure
        self.verify_block_structure(block).await?;
        
        // Verify block timing
        self.verify_block_timing(block).await?;
        
        // Verify block hash
        self.verify_block_hash(block).await?;
        
        Ok(())
    }

    /// Calculate block hash
    fn calculate_block_hash(&self, state: &ConsensusState<E>) -> Result<E::Fr, ConsensusError> {
        let mut hasher = Sha3_256::new();
        
        // Add block components to hash
        hasher.update(&state.height.to_le_bytes());
        hasher.update(&state.last_block_hash.to_bytes());
        
        // Convert hash to field element
        let hash = hasher.finalize();
        let hash_fr = E::Fr::from_random_bytes(&hash)
            .ok_or_else(|| ConsensusError::InvalidBlock("Invalid hash conversion".to_string()))?;
        
        Ok(hash_fr)
    }

    /// Verify block structure
    async fn verify_block_structure(&self, block: &Block<E>) -> Result<(), ConsensusError> {
        // Check height continuity
        let state = self.state.read().await;
        if block.height != state.height + 1 {
            return Err(ConsensusError::InvalidBlock(
                "Invalid block height".to_string()
            ));
        }
        
        // Check previous hash
        if block.prev_hash != state.last_block_hash {
            return Err(ConsensusError::InvalidBlock(
                "Invalid previous hash".to_string()
            ));
        }
        
        Ok(())
    }

    /// Verify block timing
    async fn verify_block_timing(&self, block: &Block<E>) -> Result<(), ConsensusError> {
        let last_time = *self.last_block_time.read().await;
        
        // Check minimum block time
        if block.timestamp < last_time + self.config.block_time {
            return Err(ConsensusError::InvalidBlock(
                "Block time too early".to_string()
            ));
        }
        
        // Check maximum block time
        if block.timestamp > last_time + (self.config.block_time * 2) {
            return Err(ConsensusError::InvalidBlock(
                "Block time too late".to_string()
            ));
        }
        
        Ok(())
    }

    /// Verify block hash
    async fn verify_block_hash(&self, block: &Block<E>) -> Result<(), ConsensusError> {
        let state = self.state.read().await;
        let calculated_hash = self.calculate_block_hash(&state)?;
        
        if block.hash != calculated_hash {
            return Err(ConsensusError::InvalidBlock(
                "Invalid block hash".to_string()
            ));
        }
        
        Ok(())
    }

    /// Get current time
    fn current_time(&self) -> Result<u64, ConsensusError> {
        SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .map(|d| d.as_secs())
            .map_err(|e| ConsensusError::StateTransitionError(
                format!("Time error: {}", e)
            ))
    }

    /// Verify block time
    async fn verify_block_time(&self, current_time: u64) -> Result<(), ConsensusError> {
        let last_time = *self.last_block_time.read().await;
        
        if current_time < last_time + self.config.block_time {
            return Err(ConsensusError::StateTransitionError(
                "Block time too early".to_string()
            ));
        }
        
        Ok(())
    }
}
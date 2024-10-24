use super::types::{Validator, ValidatorId, ValidatorSet, ValidatorPerformance};
use super::errors::ConsensusError;
use ark_ec::PairingEngine;
use ark_ff::Field;
use std::sync::Arc;
use tokio::sync::RwLock;

/// Validator manager for handling validator-related operations
pub struct ValidatorManager<E: PairingEngine> {
    /// Set of current validators
    validators: Arc<RwLock<ValidatorSet<E>>>,
    
    /// Minimum stake requirement
    min_stake: u64,
    
    /// Maximum validators allowed
    max_validators: usize,
}

impl<E: PairingEngine> ValidatorManager<E> {
    /// Create new validator manager
    pub fn new(min_stake: u64, max_validators: usize) -> Self {
        Self {
            validators: Arc::new(RwLock::new(ValidatorSet::new())),
            min_stake,
            max_validators,
        }
    }

    /// Register new validator
    pub async fn register_validator(
        &self,
        id: ValidatorId,
        stake: u64,
        identity_commitment: E::Fr,
    ) -> Result<(), ConsensusError> {
        // Check stake requirement
        if stake < self.min_stake {
            return Err(ConsensusError::InsufficientStake(stake));
        }

        let mut validators = self.validators.write().await;
        
        // Check maximum validator limit
        if validators.len() >= self.max_validators {
            return Err(ConsensusError::InvalidValidatorSet(
                "Maximum validator limit reached".to_string()
            ));
        }

        // Create new validator
        let validator = Validator {
            id: id.clone(),
            stake,
            identity_commitment,
            last_block: 0,
            performance: ValidatorPerformance::default(),
        };

        // Add to validator set
        validators.add_validator(validator);

        Ok(())
    }

    /// Update validator stake
    pub async fn update_stake(
        &self,
        id: &ValidatorId,
        new_stake: u64,
    ) -> Result<(), ConsensusError> {
        let mut validators = self.validators.write().await;
        
        if let Some(validator) = validators.get_validator_mut(id) {
            if new_stake < self.min_stake {
                return Err(ConsensusError::InsufficientStake(new_stake));
            }
            validator.stake = new_stake;
            Ok(())
        } else {
            Err(ConsensusError::InvalidValidatorSet(
                "Validator not found".to_string()
            ))
        }
    }

    /// Update validator performance
    pub async fn update_performance(
        &self,
        id: &ValidatorId,
        produced_block: bool,
    ) -> Result<(), ConsensusError> {
        let mut validators = self.validators.write().await;
        
        if let Some(validator) = validators.get_validator_mut(id) {
            if produced_block {
                validator.performance.blocks_produced += 1;
            } else {
                validator.performance.blocks_missed += 1;
            }
            
            // Update uptime
            let total_blocks = validator.performance.blocks_produced + validator.performance.blocks_missed;
            validator.performance.uptime = validator.performance.blocks_produced as f64 / total_blocks as f64;
            
            Ok(())
        } else {
            Err(ConsensusError::InvalidValidatorSet(
                "Validator not found".to_string()
            ))
        }
    }

    /// Remove validator
    pub async fn remove_validator(&self, id: &ValidatorId) -> Result<(), ConsensusError> {
        let mut validators = self.validators.write().await;
        validators.remove_validator(id);
        Ok(())
    }

    /// Get validator by ID
    pub async fn get_validator(&self, id: &ValidatorId) -> Option<Validator<E>> {
        let validators = self.validators.read().await;
        validators.get_validator(id).cloned()
    }

    /// Get all validators
    pub async fn get_all_validators(&self) -> ValidatorSet<E> {
        self.validators.read().await.clone()
    }
}
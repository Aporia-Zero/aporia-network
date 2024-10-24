use ark_ec::PairingEngine;
use std::sync::Arc;
use tokio::sync::RwLock;
use std::error::Error;
use std::fmt;

pub mod merkle_tree;
pub mod account;
pub mod transaction;
pub mod storage;
pub mod transition;
pub mod types;

pub use types::{State, StateRoot, StateUpdate, Account, AccountId};
pub use storage::StateStorage;
pub use transition::StateTransition;

#[derive(Debug)]
pub enum StateError {
    StorageError(String),
    MerkleError(String),
    TransitionError(String),
    ValidationError(String),
    AccountError(String),
    SerializationError(String),
}

impl fmt::Display for StateError {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            StateError::StorageError(msg) => write!(f, "Storage error: {}", msg),
            StateError::MerkleError(msg) => write!(f, "Merkle tree error: {}", msg),
            StateError::TransitionError(msg) => write!(f, "Transition error: {}", msg),
            StateError::ValidationError(msg) => write!(f, "Validation error: {}", msg),
            StateError::AccountError(msg) => write!(f, "Account error: {}", msg),
            StateError::SerializationError(msg) => write!(f, "Serialization error: {}", msg),
        }
    }
}

impl Error for StateError {}

/// State manager for handling blockchain state
pub struct StateManager<E: PairingEngine> {
    /// Current state
    state: Arc<RwLock<State<E>>>,
    
    /// State storage
    storage: Arc<RwLock<Box<dyn StateStorage<E>>>>,
    
    /// State transition handler
    transition_handler: StateTransition<E>,
}

impl<E: PairingEngine> StateManager<E> {
    /// Create new state manager
    pub fn new(storage: Box<dyn StateStorage<E>>) -> Self {
        Self {
            state: Arc::new(RwLock::new(State::new())),
            storage: Arc::new(RwLock::new(storage)),
            transition_handler: StateTransition::new(),
        }
    }

    /// Initialize state manager
    pub async fn initialize(&self) -> Result<(), StateError> {
        // Load initial state from storage
        let initial_state = self.storage.read().await.load_state()
            .map_err(|e| StateError::StorageError(e.to_string()))?;
        
        // Set initial state
        *self.state.write().await = initial_state;
        
        Ok(())
    }

    /// Apply state update
    pub async fn apply_update(&self, update: StateUpdate<E>) -> Result<StateRoot<E>, StateError> {
        // Validate update
        self.validate_update(&update).await?;
        
        // Apply transition
        let new_state = self.transition_handler.apply_update(
            &self.state.read().await,
            update.clone(),
        )?;
        
        // Save new state
        self.storage.write().await.save_state(&new_state)
            .map_err(|e| StateError::StorageError(e.to_string()))?;
        
        // Update current state
        *self.state.write().await = new_state.clone();
        
        Ok(new_state.root())
    }

    /// Get current state root
    pub async fn get_state_root(&self) -> StateRoot<E> {
        self.state.read().await.root()
    }

    /// Get account state
    pub async fn get_account(&self, id: &AccountId) -> Result<Option<Account<E>>, StateError> {
        self.state.read().await.get_account(id)
    }

    /// Validate state update
    async fn validate_update(&self, update: &StateUpdate<E>) -> Result<(), StateError> {
        // Verify update signature
        update.verify_signature()
            .map_err(|e| StateError::ValidationError(e.to_string()))?;
        
        // Verify state transition is valid
        self.transition_handler.validate_update(
            &self.state.read().await,
            update,
        )?;
        
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use ark_bls12_381::Bls12_381;
    use crate::state::storage::MemoryStorage;

    #[tokio::test]
    async fn test_state_initialization() {
        let storage = Box::new(MemoryStorage::<Bls12_381>::new());
        let state_manager = StateManager::new(storage);
        
        assert!(state_manager.initialize().await.is_ok());
    }

    #[tokio::test]
    async fn test_state_update() {
        let storage = Box::new(MemoryStorage::<Bls12_381>::new());
        let state_manager = StateManager::new(storage);
        state_manager.initialize().await.unwrap();
        
        let update = StateUpdate::new_test_update();
        let result = state_manager.apply_update(update).await;
        assert!(result.is_ok());
    }
}
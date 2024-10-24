use super::{Account, AccountId, StateError};
use crate::crypto::merkle_tree::MerkleTree;
use ark_ec::PairingEngine;
use ark_ff::Field;
use ark_serialize::{CanonicalDeserialize, CanonicalSerialize};
use std::collections::HashMap;

/// Global state structure
#[derive(Clone)]
pub struct State<E: PairingEngine> {
    /// Account states
    pub accounts: HashMap<AccountId, Account<E>>,
    
    /// State root
    pub root: E::Fr,
    
    /// State version
    pub version: u64,
    
    /// Block height
    pub block_height: u64,
    
    /// Timestamp
    pub timestamp: u64,
}

impl<E: PairingEngine> State<E> {
    /// Create new state
    pub fn new() -> Self {
        Self {
            accounts: HashMap::new(),
            root: E::Fr::zero(),
            version: 0,
            block_height: 0,
            timestamp: 0,
        }
    }

    /// Get account by ID
    pub fn get_account(&self, id: &AccountId) -> Option<Account<E>> {
        self.accounts.get(id).cloned()
    }

    /// Set account
    pub fn set_account(&mut self, account: Account<E>) {
        self.accounts.insert(account.id.clone(), account);
    }

    /// Remove account
    pub fn remove_account(&mut self, id: &AccountId) {
        self.accounts.remove(id);
    }

    /// Calculate state root
    pub fn calculate_root(
        &self,
        modified_accounts: &HashMap<AccountId, Account<E>>,
    ) -> Result<E::Fr, StateError> {
        let mut merkle_tree = MerkleTree::new(256); // 256-bit security
        
        // Add all accounts to Merkle tree
        for (id, account) in self.accounts.iter() {
            // Check if account was modified
            let account = modified_accounts
                .get(id)
                .unwrap_or(account);
            
            let account_bytes = account.serialize()?;
            merkle_tree.update(&id.0, &account_bytes)?;
        }
        
        // Add new accounts that weren't in original state
        for (id, account) in modified_accounts {
            if !self.accounts.contains_key(id) {
                let account_bytes = account.serialize()?;
                merkle_tree.update(&id.0, &account_bytes)?;
            }
        }
        
        Ok(merkle_tree.root())
    }

    /// Update state with modified accounts
    pub fn apply_modifications(
        &mut self,
        modified_accounts: HashMap<AccountId, Account<E>>,
    ) -> Result<(), StateError> {
        // Calculate new root
        let new_root = self.calculate_root(&modified_accounts)?;
        
        // Update accounts
        self.accounts.extend(modified_accounts);
        
        // Update state metadata
        self.root = new_root;
        self.version += 1;
        
        Ok(())
    }

    /// Get state proof for account
    pub fn get_account_proof(
        &self,
        id: &AccountId,
    ) -> Result<StateProof<E>, StateError> {
        let mut merkle_tree = MerkleTree::new(256);
        
        // Add all accounts to Merkle tree
        for (acc_id, account) in &self.accounts {
            let account_bytes = account.serialize()?;
            merkle_tree.update(&acc_id.0, &account_bytes)?;
        }
        
        // Generate proof
        let proof = merkle_tree.get_proof(&id.0)?;
        
        Ok(StateProof {
            account_id: id.clone(),
            account: self.get_account(id),
            merkle_proof: proof,
            root: self.root,
        })
    }

    /// Verify state proof
    pub fn verify_proof(&self, proof: &StateProof<E>) -> Result<bool, StateError> {
        let mut merkle_tree = MerkleTree::new(256);
        
        if let Some(account) = &proof.account {
            let account_bytes = account.serialize()?;
            merkle_tree.verify_proof(
                &proof.account_id.0,
                &account_bytes,
                &proof.merkle_proof,
            )?;
        }
        
        Ok(proof.root == self.root)
    }

    /// Serialize state
    pub fn serialize(&self) -> Result<Vec<u8>, StateError> {
        let mut bytes = Vec::new();
        
        // Serialize metadata
        self.version.serialize(&mut bytes)
            .map_err(|e| StateError::SerializationError(e.to_string()))?;
        self.block_height.serialize(&mut bytes)
            .map_err(|e| StateError::SerializationError(e.to_string()))?;
        self.timestamp.serialize(&mut bytes)
            .map_err(|e| StateError::SerializationError(e.to_string()))?;
        self.root.serialize(&mut bytes)
            .map_err(|e| StateError::SerializationError(e.to_string()))?;
        
        // Serialize accounts
        (self.accounts.len() as u64).serialize(&mut bytes)
            .map_err(|e| StateError::SerializationError(e.to_string()))?;
        
        for (id, account) in &self.accounts {
            id.0.serialize(&mut bytes)
                .map_err(|e| StateError::SerializationError(e.to_string()))?;
            account.serialize(&mut bytes)
                .map_err(|e| StateError::SerializationError(e.to_string()))?;
        }
        
        Ok(bytes)
    }

    /// Deserialize state
    pub fn deserialize(bytes: &[u8]) -> Result<Self, StateError> {
        let mut offset = 0;
        
        // Deserialize metadata
        let version: u64 = CanonicalDeserialize::deserialize(&bytes[offset..])
            .map_err(|e| StateError::SerializationError(e.to_string()))?;
        offset += std::mem::size_of::<u64>();
        
        let block_height: u64 = CanonicalDeserialize::deserialize(&bytes[offset..])
            .map_err(|e| StateError::SerializationError(e.to_string()))?;
        offset += std::mem::size_of::<u64>();
        
        let timestamp: u64 = CanonicalDeserialize::deserialize(&bytes[offset..])
            .map_err(|e| StateError::SerializationError(e.to_string()))?;
        offset += std::mem::size_of::<u64>();
        
        let root: E::Fr = CanonicalDeserialize::deserialize(&bytes[offset..])
            .map_err(|e| StateError::SerializationError(e.to_string()))?;
        offset += std::mem::size_of::<E::Fr>();
        
        // Deserialize accounts
        let account_count: u64 = CanonicalDeserialize::deserialize(&bytes[offset..])
            .map_err(|e| StateError::SerializationError(e.to_string()))?;
        offset += std::mem::size_of::<u64>();
        
        let mut accounts = HashMap::new();
        for _ in 0..account_count {
            let id_bytes: Vec<u8> = CanonicalDeserialize::deserialize(&bytes[offset..])
                .map_err(|e| StateError::SerializationError(e.to_string()))?;
            offset += id_bytes.len();
            
            let account: Account<E> = Account::deserialize(&bytes[offset..])
                .map_err(|e| StateError::SerializationError(e.to_string()))?;
            offset += account.serialized_size();
            
            accounts.insert(AccountId(id_bytes), account);
        }
        
        Ok(Self {
            accounts,
            root,
            version,
            block_height,
            timestamp,
        })
    }
}

/// State update structure
#[derive(Clone, Debug)]
pub struct StateUpdate<E: PairingEngine> {
    /// Block height
    pub block_height: u64,
    
    /// Timestamp
    pub timestamp: u64,
    
    /// Modified accounts
    pub modified_accounts: HashMap<AccountId, Account<E>>,
    
    /// Previous state root
    pub previous_root: E::Fr,
    
    /// New state root
    pub new_root: E::Fr,
}

impl<E: PairingEngine> StateUpdate<E> {
    /// Create new state update
    pub fn new(
        block_height: u64,
        timestamp: u64,
        modified_accounts: HashMap<AccountId, Account<E>>,
        previous_root: E::Fr,
        new_root: E::Fr,
    ) -> Self {
        Self {
            block_height,
            timestamp,
            modified_accounts,
            previous_root,
            new_root,
        }
    }

    /// Verify update validity
    pub fn verify(&self, state: &State<E>) -> Result<bool, StateError> {
        // Verify previous root matches
        if self.previous_root != state.root {
            return Ok(false);
        }
        
        // Calculate expected new root
        let calculated_root = state.calculate_root(&self.modified_accounts)?;
        
        Ok(calculated_root == self.new_root)
    }
}

/// State proof structure
#[derive(Clone, Debug)]
pub struct StateProof<E: PairingEngine> {
    /// Account ID
    pub account_id: AccountId,
    
    /// Account data (if exists)
    pub account: Option<Account<E>>,
    
    /// Merkle proof
    pub merkle_proof: Vec<E::Fr>,
    
    /// State root
    pub root: E::Fr,
}

#[cfg(test)]
mod tests {
    use super::*;
    use ark_bls12_381::Bls12_381;
    use crate::crypto::keys::KeyPair;

    #[test]
    fn test_state_creation() {
        let state = State::<Bls12_381>::new();
        assert_eq!(state.version, 0);
        assert_eq!(state.block_height, 0);
        assert!(state.accounts.is_empty());
    }

    #[test]
    fn test_account_management() {
        let mut state = State::<Bls12_381>::new();
        let id = AccountId(vec![1, 2, 3]);
        
        let account = Account::new(
            id.clone(),
            Bls12_381::G1Projective::prime_subgroup_generator(),
        );
        
        state.set_account(account.clone());
        assert_eq!(state.get_account(&id).unwrap().id, id);
        
        state.remove_account(&id);
        assert!(state.get_account(&id).is_none());
    }

    #[test]
    fn test_state_root_calculation() {
        let mut state = State::<Bls12_381>::new();
        let mut modified_accounts = HashMap::new();
        
        // Add some accounts
        for i in 0..3 {
            let id = AccountId(vec![i]);
            let account = Account::new(
                id.clone(),
                Bls12_381::G1Projective::prime_subgroup_generator(),
            );
            modified_accounts.insert(id, account);
        }
        
        let root = state.calculate_root(&modified_accounts).unwrap();
        assert!(!root.is_zero());
    }

    #[test]
    fn test_state_proof() {
        let mut state = State::<Bls12_381>::new();
        let id = AccountId(vec![1, 2, 3]);
        
        let account = Account::new(
            id.clone(),
            Bls12_381::G1Projective::prime_subgroup_generator(),
        );
        state.set_account(account);
        
        let proof = state.get_account_proof(&id).unwrap();
        assert!(state.verify_proof(&proof).unwrap());
    }

    #[test]
    fn test_state_serialization() {
        let mut state = State::<Bls12_381>::new();
        let id = AccountId(vec![1, 2, 3]);
        
        let account = Account::new(
            id.clone(),
            Bls12_381::G1Projective::prime_subgroup_generator(),
        );
        state.set_account(account);
        
        let bytes = state.serialize().unwrap();
        let deserialized = State::deserialize(&bytes).unwrap();
        
        assert_eq!(state.version, deserialized.version);
        assert_eq!(state.root, deserialized.root);
        assert_eq!(state.accounts.len(), deserialized.accounts.len());
    }

    #[test]
    fn test_state_update() {
        let mut state = State::<Bls12_381>::new();
        let mut modified_accounts = HashMap::new();
        
        let id = AccountId(vec![1, 2, 3]);
        let account = Account::new(
            id.clone(),
            Bls12_381::G1Projective::prime_subgroup_generator(),
        );
        modified_accounts.insert(id, account);
        
        let previous_root = state.root;
        let new_root = state.calculate_root(&modified_accounts).unwrap();
        
        let update = StateUpdate::new(
            1,
            1000,
            modified_accounts.clone(),
            previous_root,
            new_root,
        );
        
        assert!(update.verify(&state).unwrap());
    }
}
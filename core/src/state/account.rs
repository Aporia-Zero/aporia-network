use super::StateError;
use crate::crypto::keys::KeyPair;
use ark_ec::PairingEngine;
use ark_ff::Field;
use ark_serialize::{CanonicalDeserialize, CanonicalSerialize};
use std::collections::HashMap;

/// Account identifier
#[derive(Clone, Debug, Hash, Eq, PartialEq)]
pub struct AccountId(pub Vec<u8>);

/// Account state
#[derive(Clone, Debug)]
pub struct Account<E: PairingEngine> {
    /// Account ID
    pub id: AccountId,
    
    /// Account nonce
    pub nonce: u64,
    
    /// Account balance
    pub balance: u64,
    
    /// Account public key
    pub public_key: E::G1Projective,
    
    /// Account state root (for smart contracts)
    pub state_root: E::Fr,
    
    /// Account code hash (for smart contracts)
    pub code_hash: Option<E::Fr>,
    
    /// Account storage
    pub storage: HashMap<E::Fr, E::Fr>,
}

impl<E: PairingEngine> Account<E> {
    /// Create new account
    pub fn new(id: AccountId, public_key: E::G1Projective) -> Self {
        Self {
            id,
            nonce: 0,
            balance: 0,
            public_key,
            state_root: E::Fr::zero(),
            code_hash: None,
            storage: HashMap::new(),
        }
    }

    /// Create new contract account
    pub fn new_contract(
        id: AccountId,
        code_hash: E::Fr,
        creator_key: E::G1Projective,
    ) -> Self {
        Self {
            id,
            nonce: 0,
            balance: 0,
            public_key: creator_key,
            state_root: E::Fr::zero(),
            code_hash: Some(code_hash),
            storage: HashMap::new(),
        }
    }

    /// Increment account nonce
    pub fn increment_nonce(&mut self) {
        self.nonce += 1;
    }

    /// Update account balance
    pub fn update_balance(&mut self, amount: i64) -> Result<(), StateError> {
        if amount < 0 && self.balance < (-amount) as u64 {
            return Err(StateError::AccountError("Insufficient balance".to_string()));
        }
        
        self.balance = ((self.balance as i64) + amount) as u64;
        Ok(())
    }

    /// Set storage value
    pub fn set_storage(&mut self, key: E::Fr, value: E::Fr) {
        self.storage.insert(key, value);
    }

    /// Get storage value
    pub fn get_storage(&self, key: &E::Fr) -> Option<E::Fr> {
        self.storage.get(key).copied()
    }

    /// Check if account is a contract
    pub fn is_contract(&self) -> bool {
        self.code_hash.is_some()
    }

    /// Serialize account state
    pub fn serialize(&self) -> Result<Vec<u8>, StateError> {
        let mut bytes = Vec::new();
        
        // Serialize basic fields
        self.id.0.serialize(&mut bytes)
            .map_err(|e| StateError::SerializationError(e.to_string()))?;
        self.nonce.serialize(&mut bytes)
            .map_err(|e| StateError::SerializationError(e.to_string()))?;
        self.balance.serialize(&mut bytes)
            .map_err(|e| StateError::SerializationError(e.to_string()))?;
        self.public_key.serialize(&mut bytes)
            .map_err(|e| StateError::SerializationError(e.to_string()))?;
        self.state_root.serialize(&mut bytes)
            .map_err(|e| StateError::SerializationError(e.to_string()))?;
        
        // Serialize optional code hash
        if let Some(code_hash) = &self.code_hash {
            true.serialize(&mut bytes)
                .map_err(|e| StateError::SerializationError(e.to_string()))?;
            code_hash.serialize(&mut bytes)
                .map_err(|e| StateError::SerializationError(e.to_string()))?;
        } else {
            false.serialize(&mut bytes)
                .map_err(|e| StateError::SerializationError(e.to_string()))?;
        }
        
        // Serialize storage
        (self.storage.len() as u64).serialize(&mut bytes)
            .map_err(|e| StateError::SerializationError(e.to_string()))?;
        
        for (key, value) in &self.storage {
            key.serialize(&mut bytes)
                .map_err(|e| StateError::SerializationError(e.to_string()))?;
            value.serialize(&mut bytes)
                .map_err(|e| StateError::SerializationError(e.to_string()))?;
        }
        
        Ok(bytes)
    }

    /// Deserialize account state
    pub fn deserialize(bytes: &[u8]) -> Result<Self, StateError> {
        let mut offset = 0;
        
        // Deserialize basic fields
        let id_bytes: Vec<u8> = CanonicalDeserialize::deserialize(&bytes[offset..])
            .map_err(|e| StateError::SerializationError(e.to_string()))?;
        offset += id_bytes.serialized_size();
        
        let nonce: u64 = CanonicalDeserialize::deserialize(&bytes[offset..])
            .map_err(|e| StateError::SerializationError(e.to_string()))?;
        offset += std::mem::size_of::<u64>();
        
        let balance: u64 = CanonicalDeserialize::deserialize(&bytes[offset..])
            .map_err(|e| StateError::SerializationError(e.to_string()))?;
        offset += std::mem::size_of::<u64>();
        
        let public_key: E::G1Projective = CanonicalDeserialize::deserialize(&bytes[offset..])
            .map_err(|e| StateError::SerializationError(e.to_string()))?;
        offset += public_key.serialized_size();
        
        let state_root: E::Fr = CanonicalDeserialize::deserialize(&bytes[offset..])
            .map_err(|e| StateError::SerializationError(e.to_string()))?;
        offset += state_root.serialized_size();
        
        // Deserialize optional code hash
        let has_code: bool = CanonicalDeserialize::deserialize(&bytes[offset..])
            .map_err(|e| StateError::SerializationError(e.to_string()))?;
        offset += 1;
        
        let code_hash = if has_code {
            let hash: E::Fr = CanonicalDeserialize::deserialize(&bytes[offset..])
                .map_err(|e| StateError::SerializationError(e.to_string()))?;
            offset += hash.serialized_size();
            Some(hash)
        } else {
            None
        };
        
        // Deserialize storage
        let storage_len: u64 = CanonicalDeserialize::deserialize(&bytes[offset..])
            .map_err(|e| StateError::SerializationError(e.to_string()))?;
        offset += std::mem::size_of::<u64>();
        
        let mut storage = HashMap::new();
        for _ in 0..storage_len {
            let key: E::Fr = CanonicalDeserialize::deserialize(&bytes[offset..])
                .map_err(|e| StateError::SerializationError(e.to_string()))?;
            offset += key.serialized_size();
            
            let value: E::Fr = CanonicalDeserialize::deserialize(&bytes[offset..])
                .map_err(|e| StateError::SerializationError(e.to_string()))?;
            offset += value.serialized_size();
            
            storage.insert(key, value);
        }
        
        Ok(Self {
            id: AccountId(id_bytes),
            nonce,
            balance,
            public_key,
            state_root,
            code_hash,
            storage,
        })
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use ark_bls12_381::Bls12_381;
    use rand::thread_rng;

    #[test]
    fn test_account_creation() {
        let id = AccountId(vec![1, 2, 3]);
        let g = Bls12_381::G1Projective::prime_subgroup_generator();
        let account = Account::<Bls12_381>::new(id.clone(), g);
        
        assert_eq!(account.id, id);
        assert_eq!(account.nonce, 0);
        assert_eq!(account.balance, 0);
        assert!(!account.is_contract());
    }

    #[test]
    fn test_contract_account() {
        let id = AccountId(vec![1, 2, 3]);
        let g = Bls12_381::G1Projective::prime_subgroup_generator();
        let code_hash = Bls12_381::Fr::rand(&mut thread_rng());
        
        let account = Account::<Bls12_381>::new_contract(id.clone(), code_hash, g);
        assert!(account.is_contract());
        assert_eq!(account.code_hash, Some(code_hash));
    }

    #[test]
    fn test_account_balance() {
        let id = AccountId(vec![1, 2, 3]);
        let g = Bls12_381::G1Projective::prime_subgroup_generator();
        let mut account = Account::<Bls12_381>::new(id, g);
        
        // Test balance updates
        assert!(account.update_balance(100).is_ok());
        assert_eq!(account.balance, 100);
        
        // Test insufficient balance
        assert!(account.update_balance(-200).is_err());
    }

    #[test]
    fn test_account_storage() {
        let id = AccountId(vec![1, 2, 3]);
        let g = Bls12_381::G1Projective::prime_subgroup_generator();
        let mut account = Account::<Bls12_381>::new(id, g);
        
        let key = Bls12_381::Fr::from(1u32);
        let value = Bls12_381::Fr::from(42u32);
        
        account.set_storage(key, value);
        assert_eq!(account.get_storage(&key), Some(value));
    }

    #[test]
    fn test_account_serialization() {
        let id = AccountId(vec![1, 2, 3]);
        let g = Bls12_381::G1Projective::prime_subgroup_generator();
        let mut account = Account::<Bls12_381>::new(id, g);
        
        // Add some data
        account.update_balance(100).unwrap();
        account.set_storage(
            Bls12_381::Fr::from(1u32),
            Bls12_381::Fr::from(42u32)
        );
        
        // Test serialization/deserialization
        let bytes = account.serialize().unwrap();
        let deserialized = Account::deserialize(&bytes).unwrap();
        
        assert_eq!(account.id.0, deserialized.id.0);
        assert_eq!(account.balance, deserialized.balance);
        assert_eq!(account.storage, deserialized.storage);
    }
}
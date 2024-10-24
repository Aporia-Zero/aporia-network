use super::{State, Account, AccountId, StateError};
use ark_ec::PairingEngine;
use std::collections::HashMap;
use std::path::PathBuf;
use std::sync::Arc;
use tokio::sync::RwLock;
use serde::{Serialize, Deserialize};

/// State storage interface
pub trait StateStorage<E: PairingEngine>: Send + Sync {
    /// Load state from storage
    fn load_state(&self) -> Result<State<E>, StateError>;
    
    /// Save state to storage
    fn save_state(&mut self, state: &State<E>) -> Result<(), StateError>;
    
    /// Get account from storage
    fn get_account(&self, id: &AccountId) -> Result<Option<Account<E>>, StateError>;
    
    /// Save account to storage
    fn save_account(&mut self, account: &Account<E>) -> Result<(), StateError>;
    
    /// Delete account from storage
    fn delete_account(&mut self, id: &AccountId) -> Result<(), StateError>;
    
    /// Get storage root
    fn get_storage_root(&self) -> Result<E::Fr, StateError>;
    
    /// Clear all storage
    fn clear(&mut self) -> Result<(), StateError>;
}

/// In-memory storage implementation
pub struct MemoryStorage<E: PairingEngine> {
    /// Account storage
    accounts: HashMap<AccountId, Account<E>>,
    
    /// State root
    root: E::Fr,
}

impl<E: PairingEngine> MemoryStorage<E> {
    /// Create new memory storage
    pub fn new() -> Self {
        Self {
            accounts: HashMap::new(),
            root: E::Fr::zero(),
        }
    }
}

impl<E: PairingEngine> StateStorage<E> for MemoryStorage<E> {
    fn load_state(&self) -> Result<State<E>, StateError> {
        Ok(State {
            accounts: self.accounts.clone(),
            root: self.root,
        })
    }

    fn save_state(&mut self, state: &State<E>) -> Result<(), StateError> {
        self.accounts = state.accounts.clone();
        self.root = state.root;
        Ok(())
    }

    fn get_account(&self, id: &AccountId) -> Result<Option<Account<E>>, StateError> {
        Ok(self.accounts.get(id).cloned())
    }

    fn save_account(&mut self, account: &Account<E>) -> Result<(), StateError> {
        self.accounts.insert(account.id.clone(), account.clone());
        Ok(())
    }

    fn delete_account(&mut self, id: &AccountId) -> Result<(), StateError> {
        self.accounts.remove(id);
        Ok(())
    }

    fn get_storage_root(&self) -> Result<E::Fr, StateError> {
        Ok(self.root)
    }

    fn clear(&mut self) -> Result<(), StateError> {
        self.accounts.clear();
        self.root = E::Fr::zero();
        Ok(())
    }
}

/// Persistent storage implementation using RocksDB
pub struct PersistentStorage<E: PairingEngine> {
    /// Database instance
    db: Arc<RwLock<rocksdb::DB>>,
    
    /// Database path
    path: PathBuf,
    
    /// Phantom data for generic type
    _phantom: std::marker::PhantomData<E>,
}

impl<E: PairingEngine> PersistentStorage<E> {
    /// Create new persistent storage
    pub fn new(path: PathBuf) -> Result<Self, StateError> {
        let opts = rocksdb::Options::default();
        let db = rocksdb::DB::open(&opts, &path)
            .map_err(|e| StateError::StorageError(format!("Failed to open database: {}", e)))?;
        
        Ok(Self {
            db: Arc::new(RwLock::new(db)),
            path,
            _phantom: std::marker::PhantomData,
        })
    }

    /// Get serialized key for account
    fn account_key(id: &AccountId) -> Vec<u8> {
        let mut key = Vec::with_capacity(1 + id.0.len());
        key.push(0x01); // Prefix for accounts
        key.extend_from_slice(&id.0);
        key
    }

    /// Get serialized key for state root
    fn root_key() -> Vec<u8> {
        vec![0x00] // Key for state root
    }
}

impl<E: PairingEngine> StateStorage<E> for PersistentStorage<E> {
    async fn load_state(&self) -> Result<State<E>, StateError> {
        let db = self.db.read().await;
        let mut accounts = HashMap::new();
        
        // Load root
        let root_bytes = db.get(Self::root_key())
            .map_err(|e| StateError::StorageError(format!("Failed to read root: {}", e)))?
            .unwrap_or_default();
        
        let root = if root_bytes.is_empty() {
            E::Fr::zero()
        } else {
            E::Fr::deserialize(&root_bytes[..])
                .map_err(|e| StateError::SerializationError(e.to_string()))?
        };
        
        // Load accounts
        let iter = db.iterator(rocksdb::IteratorMode::Start);
        for item in iter {
            let (key, value) = item
                .map_err(|e| StateError::StorageError(format!("Failed to read account: {}", e)))?;
            
            if key[0] == 0x01 {
                let account = Account::deserialize(&value[..])
                    .map_err(|e| StateError::SerializationError(e.to_string()))?;
                accounts.insert(account.id.clone(), account);
            }
        }
        
        Ok(State { accounts, root })
    }

    async fn save_state(&mut self, state: &State<E>) -> Result<(), StateError> {
        let mut db = self.db.write().await;
        let batch = rocksdb::WriteBatch::default();
        
        // Save root
        let mut root_bytes = Vec::new();
        state.root.serialize(&mut root_bytes)
            .map_err(|e| StateError::SerializationError(e.to_string()))?;
        batch.put(Self::root_key(), root_bytes);
        
        // Save accounts
        for account in state.accounts.values() {
            let account_bytes = account.serialize()
                .map_err(|e| StateError::SerializationError(e.to_string()))?;
            batch.put(Self::account_key(&account.id), account_bytes);
        }
        
        db.write(batch)
            .map_err(|e| StateError::StorageError(format!("Failed to write batch: {}", e)))?;
        
        Ok(())
    }

    async fn get_account(&self, id: &AccountId) -> Result<Option<Account<E>>, StateError> {
        let db = self.db.read().await;
        let key = Self::account_key(id);
        
        if let Some(bytes) = db.get(key)
            .map_err(|e| StateError::StorageError(format!("Failed to read account: {}", e)))? {
            let account = Account::deserialize(&bytes[..])
                .map_err(|e| StateError::SerializationError(e.to_string()))?;
            Ok(Some(account))
        } else {
            Ok(None)
        }
    }

    async fn save_account(&mut self, account: &Account<E>) -> Result<(), StateError> {
        let mut db = self.db.write().await;
        let key = Self::account_key(&account.id);
        let value = account.serialize()
            .map_err(|e| StateError::SerializationError(e.to_string()))?;
        
        db.put(key, value)
            .map_err(|e| StateError::StorageError(format!("Failed to write account: {}", e)))?;
        
        Ok(())
    }

    async fn delete_account(&mut self, id: &AccountId) -> Result<(), StateError> {
        let mut db = self.db.write().await;
        let key = Self::account_key(id);
        
        db.delete(key)
            .map_err(|e| StateError::StorageError(format!("Failed to delete account: {}", e)))?;
        
        Ok(())
    }

    async fn get_storage_root(&self) -> Result<E::Fr, StateError> {
        let db = self.db.read().await;
        let root_bytes = db.get(Self::root_key())
            .map_err(|e| StateError::StorageError(format!("Failed to read root: {}", e)))?
            .unwrap_or_default();
        
        if root_bytes.is_empty() {
            Ok(E::Fr::zero())
        } else {
            E::Fr::deserialize(&root_bytes[..])
                .map_err(|e| StateError::SerializationError(e.to_string()))
        }
    }

    async fn clear(&mut self) -> Result<(), StateError> {
        let db_path = self.path.clone();
        
        // Close current database
        drop(self.db.write().await);
        
        // Destroy and recreate database
        rocksdb::DB::destroy(&rocksdb::Options::default(), &db_path)
            .map_err(|e| StateError::StorageError(format!("Failed to clear database: {}", e)))?;
        
        let db = rocksdb::DB::open(&rocksdb::Options::default(), &db_path)
            .map_err(|e| StateError::StorageError(format!("Failed to recreate database: {}", e)))?;
        
        *self.db.write().await = db;
        
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use ark_bls12_381::Bls12_381;
    use tempfile::tempdir;

    #[tokio::test]
    async fn test_memory_storage() {
        let mut storage = MemoryStorage::<Bls12_381>::new();
        
        // Create test account
        let id = AccountId(vec![1, 2, 3]);
        let account = Account::new(
            id.clone(),
            Bls12_381::G1Projective::prime_subgroup_generator(),
        );
        
        // Test save and load
        storage.save_account(&account).await.unwrap();
        let loaded = storage.get_account(&id).await.unwrap().unwrap();
        
        assert_eq!(account.id, loaded.id);
        assert_eq!(account.balance, loaded.balance);
    }

    #[tokio::test]
    async fn test_persistent_storage() {
        let temp_dir = tempdir().unwrap();
        let mut storage = PersistentStorage::<Bls12_381>::new(temp_dir.path().to_path_buf()).unwrap();
        
        // Create test account
        let id = AccountId(vec![1, 2, 3]);
        let account = Account::new(
            id.clone(),
            Bls12_381::G1Projective::prime_subgroup_generator(),
        );
        
        // Test save and load
        storage.save_account(&account).await.unwrap();
        let loaded = storage.get_account(&id).await.unwrap().unwrap();
        
        assert_eq!(account.id, loaded.id);
        assert_eq!(account.balance, loaded.balance);
        
        // Test clear
        storage.clear().await.unwrap();
        assert!(storage.get_account(&id).await.unwrap().is_none());
    }
}
use super::CryptoError;
use ark_ff::Field;
use sha3::{Sha3_256, Sha3_512, Digest};
use blake2::{Blake2b512, Blake2s256};

/// Hash function configuration
#[derive(Clone)]
pub struct HashConfig {
    /// Security level in bits
    security_level: usize,
    
    /// Hash function variant
    variant: HashVariant,
}

#[derive(Clone)]
pub enum HashVariant {
    Sha3_256,
    Sha3_512,
    Blake2b,
    Blake2s,
}

impl HashConfig {
    pub fn new(security_level: usize) -> Self {
        let variant = if security_level > 256 {
            HashVariant::Sha3_512
        } else {
            HashVariant::Sha3_256
        };

        Self {
            security_level,
            variant,
        }
    }
}

/// Generic hash trait
pub trait HashFunction {
    /// Hash arbitrary data
    fn hash(&self, data: &[u8]) -> Result<Vec<u8>, CryptoError>;
    
    /// Hash to field element
    fn hash_to_field<F: Field>(&self, data: &[u8]) -> Result<F, CryptoError>;
}

/// Implementation of different hash functions
pub struct CryptoHash {
    config: HashConfig,
}

impl CryptoHash {
    pub fn new(config: HashConfig) -> Self {
        Self { config }
    }

    fn hash_with_sha3_256(&self, data: &[u8]) -> Vec<u8> {
        let mut hasher = Sha3_256::new();
        hasher.update(data);
        hasher.finalize().to_vec()
    }

    fn hash_with_sha3_512(&self, data: &[u8]) -> Vec<u8> {
        let mut hasher = Sha3_512::new();
        hasher.update(data);
        hasher.finalize().to_vec()
    }

    fn hash_with_blake2b(&self, data: &[u8]) -> Vec<u8> {
        let mut hasher = Blake2b512::new();
        hasher.update(data);
        hasher.finalize().to_vec()
    }

    fn hash_with_blake2s(&self, data: &[u8]) -> Vec<u8> {
        let mut hasher = Blake2s256::new();
        hasher.update(data);
        hasher.finalize().to_vec()
    }
}

impl HashFunction for CryptoHash {
    fn hash(&self, data: &[u8]) -> Result<Vec<u8>, CryptoError> {
        let hash = match self.config.variant {
            HashVariant::Sha3_256 => self.hash_with_sha3_256(data),
            HashVariant::Sha3_512 => self.hash_with_sha3_512(data),
            HashVariant::Blake2b => self.hash_with_blake2b(data),
            HashVariant::Blake2s => self.hash_with_blake2s(data),
        };

        Ok(hash)
    }

    fn hash_to_field<F: Field>(&self, data: &[u8]) -> Result<F, CryptoError> {
        let hash = self.hash(data)?;
        
        F::from_random_bytes(&hash).ok_or_else(|| {
            CryptoError::HashError("Failed to convert hash to field element".to_string())
        })
    }
}

/// Merkle tree hash functions
pub struct MerkleHash {
    hasher: CryptoHash,
}

impl MerkleHash {
    pub fn new(config: HashConfig) -> Self {
        Self {
            hasher: CryptoHash::new(config),
        }
    }

    /// Hash two child nodes
    pub fn hash_nodes(&self, left: &[u8], right: &[u8]) -> Result<Vec<u8>, CryptoError> {
        let mut combined = Vec::with_capacity(left.len() + right.len());
        combined.extend_from_slice(left);
        combined.extend_from_slice(right);
        self.hasher.hash(&combined)
    }

    /// Hash leaf node
    pub fn hash_leaf(&self, data: &[u8]) -> Result<Vec<u8>, CryptoError> {
        // Prefix with 0x00 to distinguish from internal nodes
        let mut prefixed = Vec::with_capacity(data.len() + 1);
        prefixed.push(0x00);
        prefixed.extend_from_slice(data);
        self.hasher.hash(&prefixed)
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use ark_bls12_381::Fr;

    #[test]
    fn test_hash_functions() {
        let config = HashConfig::new(256);
        let hasher = CryptoHash::new(config);
        
        let data = b"test data";
        let hash = hasher.hash(data).unwrap();
        assert_eq!(hash.len(), 32); // SHA3-256 output size
    }

    #[test]
    fn test_hash_to_field() {
        let config = HashConfig::new(256);
        let hasher = CryptoHash::new(config);
        
        let data = b"test data";
        let field_element: Fr = hasher.hash_to_field(data).unwrap();
        assert!(!field_element.is_zero());
    }

    #[test]
    fn test_merkle_hash() {
        let config = HashConfig::new(256);
        let merkle_hasher = MerkleHash::new(config);
        
        let left = b"left node";
        let right = b"right node";
        
        let hash = merkle_hasher.hash_nodes(left, right).unwrap();
        assert!(!hash.is_empty());
        
        let leaf_hash = merkle_hasher.hash_leaf(left).unwrap();
        assert!(!leaf_hash.is_empty());
    }
}
use super::CryptoError;
use ark_ec::{PairingEngine, ProjectiveCurve};
use ark_ff::Field;
use rand::Rng;
use std::marker::PhantomData;

/// Key pair for digital signatures
#[derive(Clone)]
pub struct KeyPair<E: PairingEngine> {
    /// Private key
    pub secret_key: E::Fr,
    
    /// Public key
    pub public_key: E::G1Projective,
}

/// Key management system
pub struct KeyManager<E: PairingEngine> {
    /// Curve type
    _engine: PhantomData<E>,
}

impl<E: PairingEngine> KeyManager<E> {
    pub fn new() -> Self {
        Self {
            _engine: PhantomData,
        }
    }

    /// Generate new key pair
    pub fn generate_keypair<R: Rng>(&self, rng: &mut R) -> KeyPair<E> {
        // Generate random secret key
        let secret_key = E::Fr::rand(rng);
        
        // Compute public key
        let public_key = E::G1Projective::prime_subgroup_generator()
            .mul(secret_key.into_repr());

        KeyPair {
            secret_key,
            public_key,
        }
    }

    /// Derive public key from secret key
    pub fn derive_public_key(&self, secret_key: &E::Fr) -> E::G1Projective {
        E::G1Projective::prime_subgroup_generator()
            .mul(secret_key.into_repr())
    }

    /// Import key pair from bytes
    pub fn import_keypair(
        &self,
        secret_bytes: &[u8],
        public_bytes: &[u8],
    ) -> Result<KeyPair<E>, CryptoError> {
        let secret_key = E::Fr::from_random_bytes(secret_bytes)
            .ok_or_else(|| CryptoError::KeyError("Invalid secret key bytes".to_string()))?;

        let public_key = E::G1Projective::from_random_bytes(public_bytes)
            .ok_or_else(|| CryptoError::KeyError("Invalid public key bytes".to_string()))?;

        Ok(KeyPair {
            secret_key,
            public_key,
        })
    }

    /// Export key pair to bytes
    pub fn export_keypair(&self, keypair: &KeyPair<E>) -> (Vec<u8>, Vec<u8>) {
        let secret_bytes = keypair.secret_key.into_repr().to_bytes_le();
        let public_bytes = keypair.public_key.into_affine().into_repr().to_bytes_le();
        
        (secret_bytes, public_bytes)
    }

    /// Verify key pair
    pub fn verify_keypair(&self, keypair: &KeyPair<E>) -> bool {
        let derived_public = self.derive_public_key(&keypair.secret_key);
        derived_public == keypair.public_key
    }
}

/// HD key derivation
pub struct HDKeyDeriver<E: PairingEngine> {
    /// Key manager
    key_manager: KeyManager<E>,
}

impl<E: PairingEngine> HDKeyDeriver<E> {
    pub fn new() -> Self {
        Self {
            key_manager: KeyManager::new(),
        }
    }

    /// Derive child key from parent
    pub fn derive_child_key(
        &self,
        parent: &KeyPair<E>,
        index: u32,
    ) -> Result<KeyPair<E>, CryptoError> {
        // Combine parent public key and index
        let mut data = Vec::new();
        data.extend_from_slice(&parent.public_key.into_affine().into_repr().to_bytes_le());
        data.extend_from_slice(&index.to_le_bytes());
        
        // Hash the combined data
        let mut hasher = sha3::Sha3_256::new();
        hasher.update(&data);
        let hash = hasher.finalize();
        
        // Generate child private key
        let child_scalar = E::Fr::from_random_bytes(&hash)
            .ok_or_else(|| CryptoError::KeyError("Invalid child key derivation".to_string()))?;
        
        let child_secret = parent.secret_key + child_scalar;
        let child_public = self.key_manager.derive_public_key(&child_secret);
        
        Ok(KeyPair {
            secret_key: child_secret,
            public_key: child_public,
        })
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use ark_bls12_381::Bls12_381;
    use rand::thread_rng;

    #[test]
    fn test_keypair_generation() {
        let key_manager = KeyManager::<Bls12_381>::new();
        let mut rng = thread_rng();
        
        let keypair = key_manager.generate_keypair(&mut rng);
        assert!(key_manager.verify_keypair(&keypair));
    }

    #[test]
    fn test_key_export_import() {
        let key_manager = KeyManager::<Bls12_381>::new();
        let mut rng = thread_rng();
        
        let original_keypair = key_manager.generate_keypair(&mut rng);
        let (secret_bytes, public_bytes) = key_manager.export_keypair(&original_keypair);
        
        let imported_keypair = key_manager.import_keypair(&secret_bytes, &public_bytes).unwrap();
        assert_eq!(original_keypair.secret_key, imported_keypair.secret_key);
        assert_eq!(original_keypair.public_key, imported_keypair.public_key);
    }

    #[test]
    fn test_hd_key_derivation() {
        let hd_deriver = HDKeyDeriver::<Bls12_381>::new();
        let key_manager = KeyManager::<Bls12_381>::new();
        let mut rng = thread_rng();
        
        let parent = key_manager.generate_keypair(&mut rng);
        let child = hd_deriver.derive_child_key(&parent, 0).unwrap();
        
        assert!(key_manager.verify_keypair(&child));
        assert_ne!(parent.secret_key, child.secret_key);
    }
}
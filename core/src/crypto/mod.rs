use ark_ec::PairingEngine;
use ark_ff::Field;
use std::error::Error;

pub mod hash;
pub mod keys;
pub mod zk;
pub mod signature;
pub mod encryption;
pub mod utils;

#[derive(Debug)]
pub enum CryptoError {
    HashError(String),
    KeyError(String),
    ProofError(String),
    SignatureError(String),
    EncryptionError(String),
    ParameterError(String),
}

impl std::fmt::Display for CryptoError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            CryptoError::HashError(msg) => write!(f, "Hash error: {}", msg),
            CryptoError::KeyError(msg) => write!(f, "Key error: {}", msg),
            CryptoError::ProofError(msg) => write!(f, "Proof error: {}", msg),
            CryptoError::SignatureError(msg) => write!(f, "Signature error: {}", msg),
            CryptoError::EncryptionError(msg) => write!(f, "Encryption error: {}", msg),
            CryptoError::ParameterError(msg) => write!(f, "Parameter error: {}", msg),
        }
    }
}

impl Error for CryptoError {}

/// Cryptographic parameters for the system
#[derive(Clone)]
pub struct CryptoParams<E: PairingEngine> {
    /// Security parameter
    pub security_level: usize,
    
    /// Pairing-friendly curve
    pub _engine: std::marker::PhantomData<E>,
    
    /// Hash function configuration
    pub hash_config: hash::HashConfig,
    
    /// Zero-knowledge proof parameters
    pub zk_params: zk::ZKParams<E>,
}

impl<E: PairingEngine> CryptoParams<E> {
    pub fn new(security_level: usize) -> Result<Self, CryptoError> {
        if security_level < 128 {
            return Err(CryptoError::ParameterError(
                "Security level must be at least 128 bits".to_string()
            ));
        }

        Ok(Self {
            security_level,
            _engine: std::marker::PhantomData,
            hash_config: hash::HashConfig::new(security_level),
            zk_params: zk::ZKParams::setup(security_level)?,
        })
    }
}

/// Initialize cryptographic subsystem
pub fn init<E: PairingEngine>(security_level: usize) -> Result<CryptoParams<E>, CryptoError> {
    CryptoParams::new(security_level)
}

#[cfg(test)]
mod tests {
    use super::*;
    use ark_bls12_381::Bls12_381;

    #[test]
    fn test_crypto_initialization() {
        let params = init::<Bls12_381>(128);
        assert!(params.is_ok());
    }

    #[test]
    fn test_invalid_security_level() {
        let params = init::<Bls12_381>(64);
        assert!(params.is_err());
    }
}
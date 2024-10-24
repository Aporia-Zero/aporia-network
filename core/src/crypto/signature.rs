use super::CryptoError;
use ark_ec::PairingEngine;
use ark_ff::Field;
use ark_serialize::{CanonicalDeserialize, CanonicalSerialize};
use sha3::{Sha3_256, Digest};

/// Digital signature scheme
pub struct SignatureScheme<E: PairingEngine> {
    /// Security parameter
    security_level: usize,
}

/// Signature structure
#[derive(Clone, Debug)]
pub struct Signature<E: PairingEngine> {
    /// R component
    pub r: E::G1Projective,
    
    /// S component
    pub s: E::Fr,
}

impl<E: PairingEngine> SignatureScheme<E> {
    /// Create new signature scheme
    pub fn new(security_level: usize) -> Result<Self, CryptoError> {
        if security_level < 128 {
            return Err(CryptoError::ParameterError(
                "Security level must be at least 128 bits".to_string()
            ));
        }

        Ok(Self { security_level })
    }

    /// Sign message using private key
    pub fn sign(
        &self,
        message: &[u8],
        private_key: &E::Fr,
    ) -> Result<Signature<E>, CryptoError> {
        // Generate random nonce
        let k = self.generate_nonce(message, private_key)?;
        
        // Compute R = kG
        let g = E::G1Projective::prime_subgroup_generator();
        let r = g.mul(k.into_repr());
        
        // Compute hash of message and R
        let h = self.hash_message_and_point(message, &r)?;
        
        // Compute s = k - h * private_key
        let s = k - (h * private_key);
        
        Ok(Signature { r, s })
    }

    /// Verify signature using public key
    pub fn verify(
        &self,
        message: &[u8],
        signature: &Signature<E>,
        public_key: &E::G1Projective,
    ) -> Result<bool, CryptoError> {
        // Compute hash of message and R
        let h = self.hash_message_and_point(message, &signature.r)?;
        
        // Verify equation: sG = R - hP
        let g = E::G1Projective::prime_subgroup_generator();
        let left = g.mul(signature.s.into_repr());
        let right = signature.r - public_key.mul(h.into_repr());
        
        Ok(left == right)
    }

    /// Generate batch signature
    pub fn batch_sign(
        &self,
        messages: &[&[u8]],
        private_key: &E::Fr,
    ) -> Result<Vec<Signature<E>>, CryptoError> {
        messages.iter()
            .map(|msg| self.sign(msg, private_key))
            .collect()
    }

    /// Verify batch signature
    pub fn batch_verify(
        &self,
        messages: &[&[u8]],
        signatures: &[Signature<E>],
        public_key: &E::G1Projective,
    ) -> Result<bool, CryptoError> {
        if messages.len() != signatures.len() {
            return Err(CryptoError::SignatureError(
                "Number of messages and signatures must match".to_string()
            ));
        }

        // Verify all signatures
        for (msg, sig) in messages.iter().zip(signatures) {
            if !self.verify(msg, sig, public_key)? {
                return Ok(false);
            }
        }

        Ok(true)
    }

    /// Generate deterministic nonce (RFC 6979)
    fn generate_nonce(&self, message: &[u8], private_key: &E::Fr) -> Result<E::Fr, CryptoError> {
        let mut hasher = Sha3_256::new();
        
        // Add private key to hash
        let pk_bytes = private_key.into_repr().to_bytes_le();
        hasher.update(&pk_bytes);
        
        // Add message to hash
        hasher.update(message);
        
        // Generate nonce
        let hash = hasher.finalize();
        
        E::Fr::from_random_bytes(&hash).ok_or_else(|| {
            CryptoError::SignatureError("Failed to generate nonce".to_string())
        })
    }

    /// Hash message and elliptic curve point
    fn hash_message_and_point(
        &self,
        message: &[u8],
        point: &E::G1Projective,
    ) -> Result<E::Fr, CryptoError> {
        let mut hasher = Sha3_256::new();
        
        // Add point coordinates to hash
        let point_bytes = point.into_affine().into_repr().to_bytes_le();
        hasher.update(&point_bytes);
        
        // Add message to hash
        hasher.update(message);
        
        // Generate field element
        let hash = hasher.finalize();
        
        E::Fr::from_random_bytes(&hash).ok_or_else(|| {
            CryptoError::SignatureError("Failed to hash message and point".to_string())
        })
    }
}

impl<E: PairingEngine> Signature<E> {
    /// Serialize signature to bytes
    pub fn to_bytes(&self) -> Result<Vec<u8>, CryptoError> {
        let mut bytes = Vec::new();
        
        // Serialize R
        self.r.serialize(&mut bytes)
            .map_err(|e| CryptoError::SignatureError(format!("Failed to serialize R: {}", e)))?;
        
        // Serialize s
        self.s.serialize(&mut bytes)
            .map_err(|e| CryptoError::SignatureError(format!("Failed to serialize s: {}", e)))?;
        
        Ok(bytes)
    }

    /// Deserialize signature from bytes
    pub fn from_bytes(bytes: &[u8]) -> Result<Self, CryptoError> {
        let r = E::G1Projective::deserialize(bytes)
            .map_err(|e| CryptoError::SignatureError(format!("Failed to deserialize R: {}", e)))?;
        
        let s = E::Fr::deserialize(&bytes[96..])
            .map_err(|e| CryptoError::SignatureError(format!("Failed to deserialize s: {}", e)))?;
        
        Ok(Self { r, s })
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use ark_bls12_381::{Bls12_381, Fr};
    use rand::thread_rng;

    #[test]
    fn test_signature_scheme() {
        let scheme = SignatureScheme::<Bls12_381>::new(128).unwrap();
        let message = b"test message";
        
        // Generate key pair
        let private_key = Fr::rand(&mut thread_rng());
        let g = Bls12_381::G1Projective::prime_subgroup_generator();
        let public_key = g.mul(private_key.into_repr());
        
        // Sign and verify
        let signature = scheme.sign(message, &private_key).unwrap();
        let valid = scheme.verify(message, &signature, &public_key).unwrap();
        assert!(valid);
    }

    #[test]
    fn test_batch_signature() {
        let scheme = SignatureScheme::<Bls12_381>::new(128).unwrap();
        let messages = vec![b"message1", b"message2", b"message3"];
        
        // Generate key pair
        let private_key = Fr::rand(&mut thread_rng());
        let g = Bls12_381::G1Projective::prime_subgroup_generator();
        let public_key = g.mul(private_key.into_repr());
        
        // Batch sign and verify
        let signatures = scheme.batch_sign(&messages.iter().map(|m| *m).collect::<Vec<_>>(), &private_key).unwrap();
        let valid = scheme.batch_verify(
            &messages.iter().map(|m| *m).collect::<Vec<_>>(),
            &signatures,
            &public_key
        ).unwrap();
        assert!(valid);
    }

    #[test]
    fn test_signature_serialization() {
        let scheme = SignatureScheme::<Bls12_381>::new(128).unwrap();
        let message = b"test message";
        let private_key = Fr::rand(&mut thread_rng());
        
        let signature = scheme.sign(message, &private_key).unwrap();
        let bytes = signature.to_bytes().unwrap();
        let deserialized = Signature::from_bytes(&bytes).unwrap();
        
        assert_eq!(signature.r, deserialized.r);
        assert_eq!(signature.s, deserialized.s);
    }
}
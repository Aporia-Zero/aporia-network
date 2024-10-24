use super::CryptoError;
use ark_ec::PairingEngine;
use ark_ff::Field;
use ark_serialize::{CanonicalDeserialize, CanonicalSerialize};
use rand::Rng;
use sha3::{Sha3_256, Digest};

/// Cryptographic utilities
pub struct CryptoUtils;

impl CryptoUtils {
    /// Generate random bytes
    pub fn random_bytes<R: Rng>(rng: &mut R, length: usize) -> Vec<u8> {
        let mut bytes = vec![0u8; length];
        rng.fill_bytes(&mut bytes);
        bytes
    }

    /// Hash to field element
    pub fn hash_to_field<F: Field>(data: &[u8]) -> Result<F, CryptoError> {
        let mut hasher = Sha3_256::new();
        hasher.update(data);
        let hash = hasher.finalize();
        
        F::from_random_bytes(&hash).ok_or_else(|| {
            CryptoError::HashError("Failed to convert hash to field element".to_string())
        })
    }

    /// Serialize field element to bytes
    pub fn serialize_field<F: Field>(field: &F) -> Result<Vec<u8>, CryptoError> {
        let mut bytes = Vec::new();
        field.serialize(&mut bytes)
            .map_err(|e| CryptoError::ParameterError(format!("Serialization error: {}", e)))?;
        Ok(bytes)
    }

    /// Deserialize field element from bytes
    pub fn deserialize_field<F: Field>(bytes: &[u8]) -> Result<F, CryptoError> {
        F::deserialize(bytes)
            .map_err(|e| CryptoError::ParameterError(format!("Deserialization error: {}", e)))
    }

    /// XOR two byte arrays
    pub fn xor_bytes(a: &[u8], b: &[u8]) -> Result<Vec<u8>, CryptoError> {
        if a.len() != b.len() {
            return Err(CryptoError::ParameterError(
                "Byte arrays must have equal length".to_string()
            ));
        }

        Ok(a.iter().zip(b.iter()).map(|(&x, &y)| x ^ y).collect())
    }

    /// Generate commitment to field element
    pub fn commit_to_field<E: PairingEngine>(
        value: &E::Fr,
        randomness: &E::Fr,
    ) -> E::G1Projective {
        let g = E::G1Projective::prime_subgroup_generator();
        let h = E::G1Projective::prime_subgroup_generator().mul(E::Fr::from(2u32).into_repr());
        
        g.mul(value.into_repr()) + h.mul(randomness.into_repr())
    }

    /// Verify field element is in range [0, max)
    pub fn verify_field_range<F: Field>(value: &F, max: &F) -> bool {
        value < max
    }

    /// Generate zero-knowledge range proof
    pub fn generate_range_proof<E: PairingEngine>(
        value: &E::Fr,
        max: &E::Fr,
        randomness: &E::Fr,
    ) -> Result<Vec<u8>, CryptoError> {
        // This is a placeholder for actual range proof implementation
        // In practice, you would use Bulletproofs or another range proof system
        
        if !Self::verify_field_range(value, max) {
            return Err(CryptoError::ParameterError(
                "Value out of range".to_string()
            ));
        }

        let mut proof = Vec::new();
        proof.extend_from_slice(&Self::serialize_field(value)?);
        proof.extend_from_slice(&Self::serialize_field(randomness)?);
        
        Ok(proof)
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use ark_bls12_381::{Bls12_381, Fr};
    use rand::thread_rng;

    #[test]
    fn test_random_bytes() {
        let mut rng = thread_rng();
        let bytes = CryptoUtils::random_bytes(&mut rng, 32);
        assert_eq!(bytes.len(), 32);
    }

    #[test]
    fn test_hash_to_field() {
        let data = b"test data";
        let result = CryptoUtils::hash_to_field::<Fr>(data);
        assert!(result.is_ok());
        let field_element = result.unwrap();
        assert!(!field_element.is_zero());
    }

    #[test]
    fn test_field_serialization() {
        let field = Fr::from(42u32);
        let bytes = CryptoUtils::serialize_field(&field).unwrap();
        let deserialized = CryptoUtils::deserialize_field::<Fr>(&bytes).unwrap();
        assert_eq!(field, deserialized);
    }

    #[test]
    fn test_commitment() {
        let value = Fr::from(42u32);
        let randomness = Fr::from(123u32);
        let commitment = CryptoUtils::commit_to_field::<Bls12_381>(&value, &randomness);
        assert!(commitment.is_in_correct_subgroup_assuming_on_curve());
    }

    #[test]
    fn test_range_proof() {
        let value = Fr::from(42u32);
        let max = Fr::from(100u32);
        let randomness = Fr::from(123u32);
        
        let proof = CryptoUtils::generate_range_proof::<Bls12_381>(&value, &max, &randomness);
        assert!(proof.is_ok());
    }

    #[test]
    fn test_xor_bytes() {
        let a = vec![1, 2, 3, 4];
        let b = vec![5, 6, 7, 8];
        let result = CryptoUtils::xor_bytes(&a, &b).unwrap();
        assert_eq!(result, vec![4, 4, 4, 12]);
    }
}
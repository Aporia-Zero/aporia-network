use ark_ec::PairingEngine;
use ark_ff::Field;
use ark_groth16::{Groth16, ProvingKey, VerifyingKey};
use ark_relations::r1cs::{ConstraintSynthesizer, ConstraintSystem};

pub mod circuit;
pub mod proof;
pub mod params;

pub use circuit::ZKCircuit;
pub use proof::{Proof, ProofSystem};
pub use params::ZKParams;

use crate::crypto::CryptoError;

/// Zero-knowledge proof system interface
pub trait ZKProver<E: PairingEngine> {
    /// Generate a proof
    fn prove<C: ConstraintSynthesizer<E::Fr>>(
        &self,
        circuit: C,
        proving_key: &ProvingKey<E>,
    ) -> Result<Proof<E>, CryptoError>;

    /// Verify a proof
    fn verify<C: ConstraintSynthesizer<E::Fr>>(
        &self,
        circuit: C,
        proof: &Proof<E>,
        verifying_key: &VerifyingKey<E>,
    ) -> Result<bool, CryptoError>;
}

/// Implementation of the zero-knowledge proof system
pub struct ZKCore<E: PairingEngine> {
    params: ZKParams<E>,
}

impl<E: PairingEngine> ZKCore<E> {
    pub fn new(params: ZKParams<E>) -> Self {
        Self { params }
    }

    /// Setup the proving system for a specific circuit
    pub fn setup<C: ConstraintSynthesizer<E::Fr>>(
        &self,
        circuit: C,
    ) -> Result<(ProvingKey<E>, VerifyingKey<E>), CryptoError> {
        let rng = &mut rand::thread_rng();
        
        let (pk, vk) = Groth16::<E>::circuit_specific_setup(circuit, rng)
            .map_err(|e| CryptoError::ProofError(format!("Setup error: {}", e)))?;

        Ok((pk, vk))
    }
}

impl<E: PairingEngine> ZKProver<E> for ZKCore<E> {
    fn prove<C: ConstraintSynthesizer<E::Fr>>(
        &self,
        circuit: C,
        proving_key: &ProvingKey<E>,
    ) -> Result<Proof<E>, CryptoError> {
        let rng = &mut rand::thread_rng();
        
        let proof = Groth16::<E>::prove(proving_key, circuit, rng)
            .map_err(|e| CryptoError::ProofError(format!("Proving error: {}", e)))?;

        Ok(Proof::new(proof))
    }

    fn verify<C: ConstraintSynthesizer<E::Fr>>(
        &self,
        circuit: C,
        proof: &Proof<E>,
        verifying_key: &VerifyingKey<E>,
    ) -> Result<bool, CryptoError> {
        let mut cs = ConstraintSystem::<E::Fr>::new_ref();
        circuit.generate_constraints(cs.clone())
            .map_err(|e| CryptoError::ProofError(format!("Constraint generation error: {}", e)))?;

        let public_inputs = cs.instance_assignment()
            .map_err(|e| CryptoError::ProofError(format!("Public input error: {}", e)))?;

        let valid = Groth16::<E>::verify(verifying_key, &public_inputs, &proof.inner)
            .map_err(|e| CryptoError::ProofError(format!("Verification error: {}", e)))?;

        Ok(valid)
    }
}
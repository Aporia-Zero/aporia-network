pub mod consensus;
pub mod crypto;
pub mod proofs;
pub mod state;

use ark_ff::Field;
use ark_ec::{PairingEngine, ProjectiveCurve};

/// Core protocol configuration
#[derive(Debug, Clone)]
pub struct CoreConfig {
    pub network_id: u64,
    pub consensus_threshold: f64,
    pub block_time: u64,
    pub max_validators: usize,
}

/// Main protocol state
pub struct Protocol<E: PairingEngine> {
    config: CoreConfig,
    state: state::State<E>,
    consensus: consensus::Consensus<E>,
}

impl<E: PairingEngine> Protocol<E> {
    pub fn new(config: CoreConfig) -> Self {
        Self {
            state: state::State::new(),
            consensus: consensus::Consensus::new(),
            config,
        }
    }

    pub fn initialize(&mut self) -> Result<(), Box<dyn std::error::Error>> {
        // Initialize protocol components
        self.state.initialize()?;
        self.consensus.initialize(&self.config)?;
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_protocol_initialization() {
        let config = CoreConfig {
            network_id: 1,
            consensus_threshold: 0.67,
            block_time: 6000,
            max_validators: 100,
        };

        let mut protocol = Protocol::new(config);
        assert!(protocol.initialize().is_ok());
    }
}
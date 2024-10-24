use super::*;
use ark_bls12_381::Bls12_381;
use std::collections::HashMap;

mod setup {
    use super::*;

    pub fn create_test_config() -> ConsensusConfig {
        ConsensusConfig {
            min_validators: 4,
            max_validators: 100,
            min_stake: 1000,
            block_time: 6000,
            epoch_length: 100,
            max_block_size: 1024 * 1024,
            selection_threshold: 0.67,
        }
    }

    pub fn create_test_validator<E: PairingEngine>(
        id: Vec<u8>,
        stake: u64,
    ) -> Validator<E> {
        Validator {
            id: ValidatorId(id),
            stake,
            identity_commitment: E::Fr::from(1u64),
            last_block: 0,
            performance: ValidatorPerformance::default(),
        }
    }
}

#[tokio::test]
async fn test_consensus_initialization() {
    let config = setup::create_test_config();
    let consensus = Consensus::<Bls12_381>::new(config);
    
    assert!(consensus.initialize().await.is_ok());
}

#[tokio::test]
async fn test_validator_registration() {
    let config = setup::create_test_config();
    let consensus = Consensus::<Bls12_381>::new(config.clone());
    consensus.initialize().await.unwrap();

    let validator = setup::create_test_validator::<Bls12_381>(
        vec![1, 2, 3],
        config.min_stake,
    );

    assert!(consensus
        .validator_manager
        .register_validator(
            validator.id.clone(),
            validator.stake,
            validator.identity_commitment,
        )
        .await
        .is_ok());
}

#[tokio::test]
async fn test_block_production() {
    let config = setup::create_test_config();
    let consensus = Consensus::<Bls12_381>::new(config.clone());
    consensus.initialize().await.unwrap();

    let validator = setup::create_test_validator::<Bls12_381>(
        vec![1, 2, 3],
        config.min_stake,
    );

    // Register validator
    consensus
        .validator_manager
        .register_validator(
            validator.id.clone(),
            validator.stake,
            validator.identity_commitment,
        )
        .await
        .unwrap();

    // Create block
    let block = consensus
        .block_producer
        .create_block(validator.id.clone(), vec![1, 2, 3])
        .await
        .unwrap();

    // Verify block
    assert!(consensus.process_block(block).await.is_ok());
}

#[tokio::test]
async fn test_validator_selection() {
    let config = setup::create_test_config();
    let consensus = Consensus::<Bls12_381>::new(config.clone());
    consensus.initialize().await.unwrap();

    // Register multiple validators
    for i in 0..5 {
        let validator = setup::create_test_validator::<Bls12_381>(
            vec![i as u8],
            config.min_stake + (i as u64 * 1000),
        );

        consensus
            .validator_manager
            .register_validator(
                validator.id.clone(),
                validator.stake,
                validator.identity_commitment,
            )
            .await
            .unwrap();
    }

    // Select validators for next epoch
    let selected = consensus.select_validators().await.unwrap();
    assert!(selected.len() >= config.min_validators);
}

#[tokio::test]
async fn test_voting_process() {
    let config = setup::create_test_config();
    let consensus = Consensus::<Bls12_381>::new(config.clone());
    consensus.initialize().await.unwrap();

    let validator = setup::create_test_validator::<Bls12_381>(
        vec![1, 2, 3],
        config.min_stake,
    );

    // Register validator
    consensus
        .validator_manager
        .register_validator(
            validator.id.clone(),
            validator.stake,
            validator.identity_commitment,
        )
        .await
        .unwrap();

    // Create block
    let block = consensus
        .block_producer
        .create_block(validator.id.clone(), vec![1, 2, 3])
        .await
        .unwrap();

    // Submit vote
    let vote_result = consensus
        .voting_manager
        .submit_vote(block.hash, validator.id.clone(), vec![1, 2, 3])
        .await;

    assert!(vote_result.is_ok());
}

#[tokio::test]
async fn test_consensus_full_cycle() {
    let config = setup::create_test_config();
    let consensus = Consensus::<Bls12_381>::new(config.clone());
    consensus.initialize().await.unwrap();

    // Register validators
    let mut validators = Vec::new();
    for i in 0..5 {
        let validator = setup::create_test_validator::<Bls12_381>(
            vec![i as u8],
            config.min_stake + (i as u64 * 1000),
        );
        validators.push(validator.clone());

        consensus
            .validator_manager
            .register_validator(
                validator.id.clone(),
                validator.stake,
                validator.identity_commitment,
            )
            .await
            .unwrap();
    }

    // Run consensus cycle
    for i in 0..3 {
        // Select validator
        let selected = consensus.select_validators().await.unwrap();
        assert!(!selected.is_empty());

        // Create block
        let validator = validators.get(i % validators.len()).unwrap();
        let block = consensus
            .block_producer
            .create_block(validator.id.clone(), vec![1, 2, 3])
            .await
            .unwrap();

        // Process block
        assert!(consensus.process_block(block.clone()).await.is_ok());

        // Submit votes
        for validator in &validators {
            let vote_result = consensus
                .voting_manager
                .submit_vote(block.hash, validator.id.clone(), vec![1, 2, 3])
                .await;
            assert!(vote_result.is_ok());
        }
    }
}
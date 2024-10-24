use std::fmt;
use std::error::Error;

#[derive(Debug)]
pub enum ConsensusError {
    /// Invalid validator identity proof
    InvalidIdentityProof(String),
    
    /// Invalid block structure
    InvalidBlock(String),
    
    /// Insufficient stake
    InsufficientStake(u64),
    
    /// Invalid validator set
    InvalidValidatorSet(String),
    
    /// State transition error
    StateTransitionError(String),
    
    /// Voting error
    VotingError(String),
    
    /// Selection error
    SelectionError(String),
    
    /// Initialization error
    InitializationError(String),
}

impl fmt::Display for ConsensusError {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            ConsensusError::InvalidIdentityProof(msg) => 
                write!(f, "Invalid identity proof: {}", msg),
            ConsensusError::InvalidBlock(msg) => 
                write!(f, "Invalid block: {}", msg),
            ConsensusError::InsufficientStake(stake) => 
                write!(f, "Insufficient stake: {}", stake),
            ConsensusError::InvalidValidatorSet(msg) => 
                write!(f, "Invalid validator set: {}", msg),
            ConsensusError::StateTransitionError(msg) => 
                write!(f, "State transition error: {}", msg),
            ConsensusError::VotingError(msg) => 
                write!(f, "Voting error: {}", msg),
            ConsensusError::SelectionError(msg) => 
                write!(f, "Selection error: {}", msg),
            ConsensusError::InitializationError(msg) => 
                write!(f, "Initialization error: {}", msg),
        }
    }
}

impl Error for ConsensusError {}
use super::{State, Account, AccountId, Transaction, TransactionType, StateError};
use crate::crypto::signature::SignatureScheme;
use ark_ec::PairingEngine;
use ark_ff::Field;
use std::collections::HashMap;

/// State transition result
#[derive(Debug)]
pub struct TransitionResult<E: PairingEngine> {
    /// New state root
    pub new_root: E::Fr,
    
    /// Modified accounts
    pub modified_accounts: HashMap<AccountId, Account<E>>,
    
    /// Computation used
    pub computation_used: u64,
    
    /// Logs generated
    pub logs: Vec<Log<E>>,
}

/// Transaction log
#[derive(Debug, Clone)]
pub struct Log<E: PairingEngine> {
    /// Event topic
    pub topic: E::Fr,
    
    /// Event data
    pub data: Vec<u8>,
    
    /// Block number
    pub block_number: u64,
    
    /// Transaction hash
    pub transaction_hash: E::Fr,
}

/// State transition handler
pub struct StateTransition<E: PairingEngine> {
    /// Signature scheme
    signature_scheme: SignatureScheme<E>,
    
    /// Minimum computation requirement
    min_computation: u64,
}

impl<E: PairingEngine> StateTransition<E> {
    /// Create new state transition handler
    pub fn new() -> Result<Self, StateError> {
        Ok(Self {
            signature_scheme: SignatureScheme::new(128)
                .map_err(|e| StateError::ValidationError(e.to_string()))?,
            min_computation: 1000, // Minimum required computation
        })
    }

    /// Apply state transition
    pub fn apply_transaction(
        &self,
        state: &State<E>,
        transaction: &Transaction<E>,
        block_number: u64,
    ) -> Result<TransitionResult<E>, StateError> {
        // Validate transaction
        self.validate_transaction(state, transaction)?;
        
        // Process transaction based on type
        let result = match transaction.tx_type {
            TransactionType::Transfer => self.process_transfer(state, transaction)?,
            TransactionType::Deploy => self.process_deploy(state, transaction)?,
            TransactionType::Call => self.process_call(state, transaction)?,
            TransactionType::CreateAccount => self.process_create_account(state, transaction)?,
            TransactionType::UpdateAccount => self.process_update_account(state, transaction)?,
        };

        // Calculate new state root
        let new_root = state.calculate_root(&result.modified_accounts)?;

        // Create logs
        let transaction_hash = transaction.hash()?;
        let mut logs = result.logs;
        logs.iter_mut().for_each(|log| {
            log.block_number = block_number;
            log.transaction_hash = transaction_hash;
        });

        Ok(TransitionResult {
            new_root,
            modified_accounts: result.modified_accounts,
            computation_used: result.computation_used,
            logs,
        })
    }

    /// Validate transaction
    fn validate_transaction(
        &self,
        state: &State<E>,
        transaction: &Transaction<E>,
    ) -> Result<(), StateError> {
        // Verify sender exists
        let sender = state.get_account(&transaction.from)
            .ok_or_else(|| StateError::ValidationError("Sender account not found".to_string()))?;

        // Verify nonce
        if transaction.nonce != sender.nonce {
            return Err(StateError::ValidationError("Invalid nonce".to_string()));
        }

        // Verify signature
        if !transaction.verify_signature(&sender.public_key)? {
            return Err(StateError::ValidationError("Invalid signature".to_string()));
        }

        // Verify computation proof
        if !transaction.verify_computation()? {
            return Err(StateError::ValidationError("Invalid computation proof".to_string()));
        }

        // Verify sufficient balance
        if transaction.value > sender.balance {
            return Err(StateError::ValidationError("Insufficient balance".to_string()));
        }

        Ok(())
    }

    /// Process transfer transaction
    fn process_transfer(
        &self,
        state: &State<E>,
        transaction: &Transaction<E>,
    ) -> Result<TransitionResult<E>, StateError> {
        let mut modified_accounts = HashMap::new();

        // Get sender account
        let mut sender = state.get_account(&transaction.from)
            .ok_or_else(|| StateError::ValidationError("Sender account not found".to_string()))?;

        // Get receiver account
        let receiver_id = transaction.to.as_ref()
            .ok_or_else(|| StateError::ValidationError("Receiver not specified".to_string()))?;
        let mut receiver = state.get_account(receiver_id)
            .ok_or_else(|| StateError::ValidationError("Receiver account not found".to_string()))?;

        // Update balances
        sender.update_balance(-(transaction.value as i64))?;
        receiver.update_balance(transaction.value as i64)?;

        // Update sender nonce
        sender.increment_nonce();

        // Store modified accounts
        modified_accounts.insert(sender.id.clone(), sender);
        modified_accounts.insert(receiver.id.clone(), receiver);

        Ok(TransitionResult {
            new_root: E::Fr::zero(), // Will be calculated later
            modified_accounts,
            computation_used: self.calculate_computation_used(transaction)?,
            logs: Vec::new(),
        })
    }

    /// Process contract deployment
    fn process_deploy(
        &self,
        state: &State<E>,
        transaction: &Transaction<E>,
    ) -> Result<TransitionResult<E>, StateError> {
        let mut modified_accounts = HashMap::new();

        // Get sender account
        let mut sender = state.get_account(&transaction.from)
            .ok_or_else(|| StateError::ValidationError("Sender account not found".to_string()))?;

        // Create contract account
        let contract_id = self.generate_contract_id(transaction)?;
        let contract_account = Account::new_contract(
            contract_id.clone(),
            self.compute_code_hash(&transaction.data)?,
            sender.public_key,
        );

        // Update sender balance and nonce
        sender.update_balance(-(transaction.value as i64))?;
        sender.increment_nonce();

        // Store modified accounts
        modified_accounts.insert(sender.id.clone(), sender);
        modified_accounts.insert(contract_id, contract_account);

        Ok(TransitionResult {
            new_root: E::Fr::zero(), // Will be calculated later
            modified_accounts,
            computation_used: self.calculate_computation_used(transaction)?,
            logs: Vec::new(),
        })
    }

    /// Process contract call
    fn process_call(
        &self,
        state: &State<E>,
        transaction: &Transaction<E>,
    ) -> Result<TransitionResult<E>, StateError> {
        let mut modified_accounts = HashMap::new();
        let mut logs = Vec::new();

        // Get sender account
        let mut sender = state.get_account(&transaction.from)
            .ok_or_else(|| StateError::ValidationError("Sender account not found".to_string()))?;

        // Get contract account
        let contract_id = transaction.to.as_ref()
            .ok_or_else(|| StateError::ValidationError("Contract not specified".to_string()))?;
        let mut contract = state.get_account(contract_id)
            .ok_or_else(|| StateError::ValidationError("Contract not found".to_string()))?;

        if !contract.is_contract() {
            return Err(StateError::ValidationError("Target is not a contract".to_string()));
        }

        // Execute contract call
        let (storage_updates, call_logs) = self.execute_contract_call(&contract, transaction)?;

        // Update contract storage
        for (key, value) in storage_updates {
            contract.set_storage(key, value);
        }

        // Update sender balance and nonce
        sender.update_balance(-(transaction.value as i64))?;
        sender.increment_nonce();

        // Store modified accounts
        modified_accounts.insert(sender.id.clone(), sender);
        modified_accounts.insert(contract.id.clone(), contract);

        logs.extend(call_logs);

        Ok(TransitionResult {
            new_root: E::Fr::zero(), // Will be calculated later
            modified_accounts,
            computation_used: self.calculate_computation_used(transaction)?,
            logs,
        })
    }

    /// Process account creation
    fn process_create_account(
        &self,
        state: &State<E>,
        transaction: &Transaction<E>,
    ) -> Result<TransitionResult<E>, StateError> {
        let mut modified_accounts = HashMap::new();

        // Verify account doesn't exist
        let account_id = transaction.to.as_ref()
            .ok_or_else(|| StateError::ValidationError("Account ID not specified".to_string()))?;
        if state.get_account(account_id).is_some() {
            return Err(StateError::ValidationError("Account already exists".to_string()));
        }

        // Get sender account
        let mut sender = state.get_account(&transaction.from)
            .ok_or_else(|| StateError::ValidationError("Sender account not found".to_string()))?;

        // Create new account
        let public_key = self.extract_public_key(&transaction.data)?;
        let new_account = Account::new(account_id.clone(), public_key);

        // Update sender nonce
        sender.increment_nonce();

        // Store modified accounts
        modified_accounts.insert(sender.id.clone(), sender);
        modified_accounts.insert(account_id.clone(), new_account);

        Ok(TransitionResult {
            new_root: E::Fr::zero(), // Will be calculated later
            modified_accounts,
            computation_used: self.calculate_computation_used(transaction)?,
            logs: Vec::new(),
        })
    }

    /// Process account update
    fn process_update_account(
        &self,
        state: &State<E>,
        transaction: &Transaction<E>,
    ) -> Result<TransitionResult<E>, StateError> {
        let mut modified_accounts = HashMap::new();

        // Get target account
        let account_id = transaction.to.as_ref()
            .ok_or_else(|| StateError::ValidationError("Account ID not specified".to_string()))?;
        let mut account = state.get_account(account_id)
            .ok_or_else(|| StateError::ValidationError("Account not found".to_string()))?;

        // Get sender account
        let mut sender = state.get_account(&transaction.from)
            .ok_or_else(|| StateError::ValidationError("Sender account not found".to_string()))?;

        // Verify sender is the account owner
        if sender.id != account.id {
            return Err(StateError::ValidationError("Not account owner".to_string()));
        }

        // Update account
        self.apply_account_updates(&mut account, &transaction.data)?;

        // Update sender nonce
        sender.increment_nonce();

        // Store modified accounts
        modified_accounts.insert(sender.id.clone(), sender);
        modified_accounts.insert(account.id.clone(), account);

        Ok(TransitionResult {
            new_root: E::Fr::zero(), // Will be calculated later
            modified_accounts,
            computation_used: self.calculate_computation_used(transaction)?,
            logs: Vec::new(),
        })
    }

    // Helper functions
    fn calculate_computation_used(&self, transaction: &Transaction<E>) -> Result<u64, StateError> {
        // Base computation cost
        let mut computation = self.min_computation;

        // Add cost based on data size
        computation += transaction.data.len() as u64 * 10;

        // Add cost based on transaction type
        computation += match transaction.tx_type {
            TransactionType::Transfer => 1000,
            TransactionType::Deploy => 50000,
            TransactionType::Call => 5000,
            TransactionType::CreateAccount => 2000,
            TransactionType::UpdateAccount => 3000,
        };

        Ok(computation)
    }

    fn generate_contract_id(&self, transaction: &Transaction<E>) -> Result<AccountId, StateError> {
        let mut hasher = sha3::Sha3_256::new();
        hasher.update(&transaction.from.0);
        hasher.update(&transaction.nonce.to_le_bytes());
        Ok(AccountId(hasher.finalize().to_vec()))
    }

    fn compute_code_hash(&self, code: &[u8]) -> Result<E::Fr, StateError> {
        let mut hasher = sha3::Sha3_256::new();
        hasher.update(code);
        let hash = hasher.finalize();

        E::Fr::from_random_bytes(&hash)
            .ok_or_else(|| StateError::ValidationError("Invalid code hash".to_string()))
    }

    fn execute_contract_call(
        &self,
        contract: &Account<E>,
        transaction: &Transaction<E>,
    ) -> Result<(HashMap<E::Fr, E::Fr>, Vec<Log<E>>), StateError> {
        // This is a placeholder for actual contract execution
        // In a real implementation, this would:
        // 1. Load and verify contract code
        // 2. Set up execution environment
        // 3. Execute contract code
        // 4. Return storage updates and logs

        Ok((HashMap::new(), Vec::new()))
    }

    fn extract_public_key(&self, data: &[u8]) -> Result<E::G1Projective, StateError> {
        // This is a placeholder for actual public key extraction
        Ok(E::G1Projective::prime_subgroup_generator())
    }

    fn apply_account_updates(&self, account: &mut Account<E>, data: &[u8]) -> Result<(), StateError> {
        // This is a placeholder for actual account updates
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use ark_bls12_381::{Bls12_381, Fr};
    use crate::crypto::signature::SignatureScheme;
    use rand::thread_rng;

    fn setup_test_state() -> State<Bls12_381> {
        let mut state = State::new();
        
        // Create test accounts with initial balance
        let sender_id = AccountId(vec![1]);
        let mut sender = Account::new(
            sender_id.clone(),
            Bls12_381::G1Projective::prime_subgroup_generator(),
        );
        sender.balance = 1000;
        state.accounts.insert(sender_id, sender);
        
        let receiver_id = AccountId(vec![2]);
        let receiver = Account::new(
            receiver_id.clone(),
            Bls12_381::G1Projective::prime_subgroup_generator(),
        );
        state.accounts.insert(receiver_id, receiver);
        
        state
    }

    fn create_signed_transaction(
        tx_type: TransactionType,
        from: AccountId,
        to: Option<AccountId>,
        value: u64,
        nonce: u64,
        private_key: &Fr,
    ) -> Transaction<Bls12_381> {
        let mut tx = Transaction::new(
            tx_type,
            from,
            to,
            value,
            nonce,
            vec![],
        );
        
        let signature_scheme = SignatureScheme::new(128).unwrap();
        tx.sign(&signature_scheme, private_key).unwrap();
        tx.add_computation_proof(vec![1, 2, 3]); // Mock proof
        
        tx
    }

    #[test]
    fn test_transfer_transaction() {
        let state_transition = StateTransition::<Bls12_381>::new().unwrap();
        let state = setup_test_state();
        let mut rng = thread_rng();
        
        let private_key = Fr::rand(&mut rng);
        let tx = create_signed_transaction(
            TransactionType::Transfer,
            AccountId(vec![1]),
            Some(AccountId(vec![2])),
            100,
            0,
            &private_key,
        );
        
        let result = state_transition.apply_transaction(&state, &tx, 1).unwrap();
        
        // Verify balances
        let sender_account = result.modified_accounts.get(&AccountId(vec![1])).unwrap();
        let receiver_account = result.modified_accounts.get(&AccountId(vec![2])).unwrap();
        
        assert_eq!(sender_account.balance, 900);
        assert_eq!(receiver_account.balance, 100);
    }

    #[test]
    fn test_deploy_contract() {
        let state_transition = StateTransition::<Bls12_381>::new().unwrap();
        let state = setup_test_state();
        let mut rng = thread_rng();
        
        let private_key = Fr::rand(&mut rng);
        let contract_code = vec![1, 2, 3, 4]; // Mock contract code
        let mut tx = Transaction::new(
            TransactionType::Deploy,
            AccountId(vec![1]),
            None,
            0,
            0,
            contract_code,
        );
        
        let signature_scheme = SignatureScheme::new(128).unwrap();
        tx.sign(&signature_scheme, &private_key).unwrap();
        tx.add_computation_proof(vec![1, 2, 3]);
        
        let result = state_transition.apply_transaction(&state, &tx, 1).unwrap();
        
        // Verify contract deployment
        assert_eq!(result.modified_accounts.len(), 2);
        let contract_account = result.modified_accounts.values()
            .find(|account| account.is_contract())
            .unwrap();
        assert!(contract_account.code_hash.is_some());
    }

    #[test]
    fn test_contract_call() {
        let state_transition = StateTransition::<Bls12_381>::new().unwrap();
        let mut state = setup_test_state();
        let mut rng = thread_rng();
        
        // Deploy contract first
        let contract_id = AccountId(vec![3]);
        let mut contract = Account::new_contract(
            contract_id.clone(),
            Fr::rand(&mut rng),
            Bls12_381::G1Projective::prime_subgroup_generator(),
        );
        state.accounts.insert(contract_id.clone(), contract);
        
        // Create contract call transaction
        let private_key = Fr::rand(&mut rng);
        let tx = create_signed_transaction(
            TransactionType::Call,
            AccountId(vec![1]),
            Some(contract_id),
            0,
            0,
            &private_key,
        );
        
        let result = state_transition.apply_transaction(&state, &tx, 1).unwrap();
        
        // Verify contract call
        assert!(result.modified_accounts.contains_key(&AccountId(vec![1])));
        assert!(result.modified_accounts.contains_key(&AccountId(vec![3])));
    }

    #[test]
    fn test_create_account() {
        let state_transition = StateTransition::<Bls12_381>::new().unwrap();
        let state = setup_test_state();
        let mut rng = thread_rng();
        
        let private_key = Fr::rand(&mut rng);
        let new_account_id = AccountId(vec![4]);
        let tx = create_signed_transaction(
            TransactionType::CreateAccount,
            AccountId(vec![1]),
            Some(new_account_id.clone()),
            0,
            0,
            &private_key,
        );
        
        let result = state_transition.apply_transaction(&state, &tx, 1).unwrap();
        
        // Verify account creation
        assert!(result.modified_accounts.contains_key(&new_account_id));
        let new_account = result.modified_accounts.get(&new_account_id).unwrap();
        assert_eq!(new_account.balance, 0);
    }

    #[test]
    fn test_invalid_nonce() {
        let state_transition = StateTransition::<Bls12_381>::new().unwrap();
        let state = setup_test_state();
        let mut rng = thread_rng();
        
        let private_key = Fr::rand(&mut rng);
        let tx = create_signed_transaction(
            TransactionType::Transfer,
            AccountId(vec![1]),
            Some(AccountId(vec![2])),
            100,
            1, // Invalid nonce
            &private_key,
        );
        
        let result = state_transition.apply_transaction(&state, &tx, 1);
        assert!(result.is_err());
    }

    #[test]
    fn test_insufficient_balance() {
        let state_transition = StateTransition::<Bls12_381>::new().unwrap();
        let state = setup_test_state();
        let mut rng = thread_rng();
        
        let private_key = Fr::rand(&mut rng);
        let tx = create_signed_transaction(
            TransactionType::Transfer,
            AccountId(vec![1]),
            Some(AccountId(vec![2])),
            2000, // More than available balance
            0,
            &private_key,
        );
        
        let result = state_transition.apply_transaction(&state, &tx, 1);
        assert!(result.is_err());
    }

    #[test]
    fn test_computation_used() {
        let state_transition = StateTransition::<Bls12_381>::new().unwrap();
        let state = setup_test_state();
        let mut rng = thread_rng();
        
        let private_key = Fr::rand(&mut rng);
        let tx = create_signed_transaction(
            TransactionType::Transfer,
            AccountId(vec![1]),
            Some(AccountId(vec![2])),
            100,
            0,
            &private_key,
        );
        
        let result = state_transition.apply_transaction(&state, &tx, 1).unwrap();
        assert!(result.computation_used >= state_transition.min_computation);
    }
}

// Additional helper methods for StateTransition
impl<E: PairingEngine> StateTransition<E> {
    /// Validate block of transactions
    pub fn validate_block(
        &self,
        state: &State<E>,
        transactions: &[Transaction<E>],
    ) -> Result<(), StateError> {
        let mut nonce_map = HashMap::new();
        
        for tx in transactions {
            // Check basic transaction validity
            self.validate_transaction(state, tx)?;
            
            // Check nonce sequence
            let nonce = nonce_map.entry(tx.from.clone()).or_insert(0);
            if tx.nonce != *nonce {
                return Err(StateError::ValidationError("Invalid nonce sequence".to_string()));
            }
            *nonce += 1;
        }
        
        Ok(())
    }

    /// Apply block of transactions
    pub fn apply_block(
        &self,
        state: &State<E>,
        transactions: &[Transaction<E>],
        block_number: u64,
    ) -> Result<TransitionResult<E>, StateError> {
        let mut modified_accounts = HashMap::new();
        let mut total_computation = 0u64;
        let mut all_logs = Vec::new();
        
        // Validate entire block first
        self.validate_block(state, transactions)?;
        
        // Apply each transaction
        for tx in transactions {
            let result = self.apply_transaction(state, tx, block_number)?;
            
            // Merge results
            modified_accounts.extend(result.modified_accounts);
            total_computation += result.computation_used;
            all_logs.extend(result.logs);
        }
        
        // Calculate final state root
        let new_root = state.calculate_root(&modified_accounts)?;
        
        Ok(TransitionResult {
            new_root,
            modified_accounts,
            computation_used: total_computation,
            logs: all_logs,
        })
    }

    /// Verify state transition
    pub fn verify_transition(
        &self,
        old_state: &State<E>,
        new_state: &State<E>,
        transactions: &[Transaction<E>],
    ) -> Result<bool, StateError> {
        // Apply transactions to old state
        let result = self.apply_block(old_state, transactions, 0)?;
        
        // Verify new state matches expected result
        if new_state.root != result.new_root {
            return Ok(false);
        }
        
        // Verify all account changes
        for (id, account) in &result.modified_accounts {
            if new_state.get_account(id) != Some(account) {
                return Ok(false);
            }
        }
        
        Ok(true)
    }
}
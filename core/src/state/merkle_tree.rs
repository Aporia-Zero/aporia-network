use super::StateError;
use crate::crypto::hash::{HashFunction, CryptoHash, HashConfig};
use ark_ec::PairingEngine;
use ark_ff::Field;
use std::collections::HashMap;

/// Merkle tree node
#[derive(Clone, Debug)]
pub struct Node<E: PairingEngine> {
    /// Node hash
    hash: E::Fr,
    
    /// Node value (if leaf)
    value: Option<Vec<u8>>,
    
    /// Left child
    left: Option<Box<Node<E>>>,
    
    /// Right child
    right: Option<Box<Node<E>>>,
}

/// Sparse Merkle tree implementation
pub struct MerkleTree<E: PairingEngine> {
    /// Root node
    root: Node<E>,
    
    /// Tree depth
    depth: usize,
    
    /// Hash function
    hasher: CryptoHash,
    
    /// Node cache
    cache: HashMap<Vec<u8>, Node<E>>,
}

impl<E: PairingEngine> MerkleTree<E> {
    /// Create new Merkle tree
    pub fn new(depth: usize) -> Self {
        let config = HashConfig::new(256);
        Self {
            root: Node {
                hash: E::Fr::zero(),
                value: None,
                left: None,
                right: None,
            },
            depth,
            hasher: CryptoHash::new(config),
            cache: HashMap::new(),
        }
    }

    /// Update leaf value
    pub fn update(&mut self, key: &[u8], value: &[u8]) -> Result<E::Fr, StateError> {
        let path = self.get_path(key);
        self.update_leaf(&mut self.root, &path, 0, value)
    }

    /// Get leaf value
    pub fn get(&self, key: &[u8]) -> Result<Option<Vec<u8>>, StateError> {
        let path = self.get_path(key);
        self.get_leaf(&self.root, &path, 0)
    }

    /// Get Merkle proof
    pub fn get_proof(&self, key: &[u8]) -> Result<MerkleProof<E>, StateError> {
        let path = self.get_path(key);
        let mut proof = Vec::new();
        self.build_proof(&self.root, &path, 0, &mut proof)?;
        Ok(MerkleProof { proof })
    }

    /// Verify Merkle proof
    pub fn verify_proof(
        &self,
        key: &[u8],
        value: &[u8],
        proof: &MerkleProof<E>,
    ) -> Result<bool, StateError> {
        let path = self.get_path(key);
        let mut current_hash = self.hash_leaf(value)?;
        
        for (i, sibling) in proof.proof.iter().enumerate() {
            let (left, right) = if path[i] {
                (sibling, &current_hash)
            } else {
                (&current_hash, sibling)
            };
            current_hash = self.hash_nodes(left, right)?;
        }
        
        Ok(current_hash == self.root.hash)
    }

    /// Update leaf node
    fn update_leaf(
        &mut self,
        node: &mut Node<E>,
        path: &[bool],
        depth: usize,
        value: &[u8],
    ) -> Result<E::Fr, StateError> {
        if depth == self.depth {
            node.value = Some(value.to_vec());
            node.hash = self.hash_leaf(value)?;
            return Ok(node.hash);
        }

        let child = if path[depth] {
            &mut node.right
        } else {
            &mut node.left
        };

        let child_hash = self.update_leaf(
            child.get_or_insert_with(|| Box::new(Node {
                hash: E::Fr::zero(),
                value: None,
                left: None,
                right: None,
            })),
            path,
            depth + 1,
            value,
        )?;

        node.hash = if path[depth] {
            self.hash_nodes(&node.left.as_ref().unwrap().hash, &child_hash)?
        } else {
            self.hash_nodes(&child_hash, &node.right.as_ref().unwrap().hash)?
        };

        Ok(node.hash)
    }

    /// Get leaf node
    fn get_leaf(
        &self,
        node: &Node<E>,
        path: &[bool],
        depth: usize,
    ) -> Result<Option<Vec<u8>>, StateError> {
        if depth == self.depth {
            return Ok(node.value.clone());
        }

        let child = if path[depth] {
            node.right.as_ref()
        } else {
            node.left.as_ref()
        };

        match child {
            Some(child) => self.get_leaf(child, path, depth + 1),
            None => Ok(None),
        }
    }

    /// Build Merkle proof
    fn build_proof(
        &self,
        node: &Node<E>,
        path: &[bool],
        depth: usize,
        proof: &mut Vec<E::Fr>,
    ) -> Result<(), StateError> {
        if depth == self.depth {
            return Ok(());
        }

        proof.push(if path[depth] {
            node.left.as_ref().unwrap().hash
        } else {
            node.right.as_ref().unwrap().hash
        });

        let child = if path[depth] {
            node.right.as_ref()
        } else {
            node.left.as_ref()
        };

        match child {
            Some(child) => self.build_proof(child, path, depth + 1, proof),
            None => Ok(()),
        }
    }

    /// Get path to leaf
    fn get_path(&self, key: &[u8]) -> Vec<bool> {
        let mut hasher = sha3::Sha3_256::new();
        hasher.update(key);
        let hash = hasher.finalize();
        
        hash.iter()
            .take(self.depth)
            .flat_map(|&byte| (0..8).map(move |i| (byte >> i) & 1 == 1))
            .collect()
    }

    /// Hash leaf node
    fn hash_leaf(&self, value: &[u8]) -> Result<E::Fr, StateError> {
        self.hasher.hash_to_field(value)
            .map_err(|e| StateError::MerkleError(e.to_string()))
    }

    /// Hash internal nodes
    fn hash_nodes(&self, left: &E::Fr, right: &E::Fr) -> Result<E::Fr, StateError> {
        let mut data = Vec::new();
        data.extend_from_slice(&left.to_repr());
        data.extend_from_slice(&right.to_repr());
        
        self.hasher.hash_to_field(&data)
            .map_err(|e| StateError::MerkleError(e.to_string()))
    }
}

/// Merkle proof structure
#[derive(Clone, Debug)]
pub struct MerkleProof<E: PairingEngine> {
    /// Proof elements
    proof: Vec<E::Fr>,
}

#[cfg(test)]
mod tests {
    use super::*;
    use ark_bls12_381::Bls12_381;

    #[test]
    fn test_merkle_tree() {
        let mut tree = MerkleTree::<Bls12_381>::new(8);
        
        let key = b"test_key";
        let value = b"test_value";
        
        // Update leaf
        let root = tree.update(key, value).unwrap();
        assert!(!root.is_zero());
        
        // Get leaf
        let retrieved = tree.get(key).unwrap().unwrap();
        assert_eq!(retrieved, value);
    }

    #[test]
    fn test_merkle_proof() {
        let mut tree = MerkleTree::<Bls12_381>::new(8);
        
        let key = b"test_key";
        let value = b"test_value";
        
        // Update and get proof
        tree.update(key, value).unwrap();
        let proof = tree.get_proof(key).unwrap();
        
        // Verify proof
        assert!(tree.verify_proof(key, value, &proof).unwrap());
    }
}
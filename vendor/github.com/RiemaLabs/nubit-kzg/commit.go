package kzg

import (
	"errors"
	"fmt"
	"hash"

	bls12381 "github.com/consensys/gnark-crypto/ecc/bls12-381"
	"github.com/consensys/gnark-crypto/ecc/bls12-381/fr"

	gokzg4844 "github.com/RiemaLabs/go-kzg-4844"
)

const (
	MaxDataLengh   = gokzg4844.ScalarsPerBlob
	CommitmentSize = gokzg4844.CompressedG1Size
	ProofSize      = gokzg4844.CompressedG1Size
)

var (
	errInvalidCommitmentSize = errors.New("invalid commitment size")
	errInvalidProofSize      = errors.New("invalid proof size")

	_ hash.Hash = (*KzgCommitter)(nil)
)

type Committer interface {
	hash.Hash
	// Commit returns the root of the tree.
	Commit(data [][]byte) ([]byte, error)
	// Verify returns an error if the proof is invalid.
	Verify(commit []byte, proof KzgOpen) error
	// Open returns the proof for the leaf at the given index.
	Open(data [][]byte, index int) (KzgProof, []byte, error)

	EmptyCommitment() []byte
	HashLeaf(ndata []byte) ([]byte, error)
	//NamespaceSize() namespace.IDSize
}

type KzgCommitter struct {
	ctx        *gokzg4844.Context
	baseHasher hash.Hash
	// NamespaceLen namespace.IDSize

	data []byte
}

func NewKzgCommitter(hasher hash.Hash) *KzgCommitter {
	ctx, err := gokzg4844.NewContext4096Secure()
	if err != nil {
		panic(err)
	}
	return &KzgCommitter{
		ctx:        ctx,
		baseHasher: hasher,
		//NamespaceLen: size,
		data: nil,
	}
}

// Commit returns the root of the tree.
// leafHashes is a list of hashes of the leaves of the tree.
// should be the same length as the number of leaves in the tree.
func (k *KzgCommitter) Commit(leafHashes [][]byte) ([]byte, error) {
	data := gokzg4844.Blob{}
	for i, h := range leafHashes {
		copy(data[i*gokzg4844.SerializedScalarSize:(i+1)*gokzg4844.SerializedScalarSize], h)
	}
	commit, err := k.ctx.BlobToKZGCommitment(&data, 0)
	if err != nil {
		return nil, err
	}
	return commit[:], nil
}

func (k *KzgCommitter) EmptyCommitment() []byte {
	empty := new(bls12381.G1Affine).Bytes()
	return empty[:]
}

// func (k *KzgCommitter) NamespaceSize() namespace.IDSize {
// 	return k.NamespaceLen
// }

func (k *KzgCommitter) Open(leafHashes [][]byte, index int) (KzgProof, []byte, error) {
	blob := gokzg4844.Blob{}
	for i, h := range leafHashes {
		copy(blob[i*gokzg4844.SerializedScalarSize:(i+1)*gokzg4844.SerializedScalarSize], h)
	}

	evaluationPoint, err := k.ctx.DomainByIndex(index)
	if err != nil {
		return nil, nil, err
	}

	inputPoint := gokzg4844.SerializeScalar(*evaluationPoint)

	proof, v, err := k.ctx.ComputeKZGProof(&blob, inputPoint, 0)
	if err != nil {
		return nil, nil, err
	}
	return proof[:], v[:], nil
}

func (k *KzgCommitter) Size() int {
	return k.baseHasher.Size()
}

func (k *KzgCommitter) CommentSize() int {
	return gokzg4844.CompressedG1Size
}

func (k *KzgCommitter) Verify(commit []byte, proof KzgOpen) error {
	if len(commit) != gokzg4844.CompressedG1Size {
		return errInvalidCommitmentSize
	}

	if len(proof.Proof()) != gokzg4844.CompressedG1Size {
		return errInvalidProofSize
	}

	var c gokzg4844.KZGCommitment
	copy(c[:], commit)

	var p gokzg4844.KZGProof
	copy(p[:], proof.Proof())

	in, err := k.ctx.DomainByIndex(proof.Index())
	if err != nil {
		return err
	}
	inputPointBytes := gokzg4844.SerializeScalar(*in)

	leaf := proof.Value()
	hashLeaf, err := k.HashLeaf(leaf[:])
	if err != nil {
		return err
	}

	var claimedValue fr.Element
	claimedValue.SetBytesCanonical(hashLeaf[:])
	claimedValueBytes := gokzg4844.SerializeScalar(claimedValue)

	return k.ctx.VerifyKZGProof(c, inputPointBytes, claimedValueBytes, p)
}

// Write writes the namespaced data to be hashed.
//
// Requires data of fixed size to match leaf or inner NMT nodes. Only a single
// write is allowed.
// It panics if more than one single write is attempted.
// If the data does not match the format of an NMT non-leaf node or leaf node, an error will be returned.
func (n *KzgCommitter) Write(data []byte) (int, error) {
	if n.data != nil {
		panic("only a single Write is allowed")
	}
	ln := len(data)
	n.data = data
	return ln, nil
}

// Sum computes the hash. Does not append the given suffix, violating the
// interface.
// It may panic if the data being hashed is invalid. This should never happen since the Write method refuses an invalid data and errors out.
func (n *KzgCommitter) Sum([]byte) []byte {
	res, err := n.HashLeaf(n.data)
	if err != nil {
		panic(err) // this should never happen since the data is already validated in the Write method
	}
	return res
}

// Reset resets the Hash to its initial state.
func (n *KzgCommitter) Reset() {
	n.data = nil // reset with an invalid node type, as zero value is a valid Leaf
	n.baseHasher.Reset()
}

// BlockSize returns the hash's underlying block size.
func (n *KzgCommitter) BlockSize() int {
	return n.baseHasher.BlockSize()
}

func (n *KzgCommitter) HashLeaf(ndata []byte) ([]byte, error) {
	h := n.baseHasher
	h.Reset()

	h.Write(ndata)
	hData := h.Sum(nil)

	var sc fr.Element
	sc.SetBytes(hData)
	res := sc.Bytes()

	return res[:], nil
}

// MustHashLeaf is a wrapper around HashLeaf that panics if an error is
// encountered. The ndata must be a valid leaf node.
func (n *KzgCommitter) MustHashLeaf(ndata []byte) []byte {
	res, err := n.HashLeaf(ndata)
	if err != nil {
		panic(err)
	}
	return res
}

// CommitmentFromBytesSlice
func CommitmentFromBytesSlice(h hash.Hash, data [][]byte) ([]byte, error) {
	if len(data) > MaxDataLengh {
		return nil, fmt.Errorf("exceeds maximum length, want: %v, got: %v", MaxDataLengh, len(data))
	}
	committer := NewKzgCommitter(h)
	hf := h
	blob := gokzg4844.Blob{}
	for i, d := range data {
		hf.Reset()

		hf.Write(d)
		hData := h.Sum(nil)

		var sc fr.Element
		sc.SetBytes(hData)
		res := sc.Bytes()

		copy(blob[i*gokzg4844.SerializedScalarSize:(i+1)*gokzg4844.SerializedScalarSize], res[:])
	}
	commit, err := committer.ctx.BlobToKZGCommitment(&blob, 0)
	if err != nil {
		return nil, err
	}
	return commit[:], nil
}

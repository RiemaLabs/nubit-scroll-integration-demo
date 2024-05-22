package kzg

import (
	"bytes"
	"encoding/json"
	"hash"

	"github.com/RiemaLabs/nubit-kzg/namespace"
	pb "github.com/RiemaLabs/nubit-kzg/proto"
)

// NamespaceRangeProof represents a namespace proof of a namespace.ID in an NMT. In case this
// proof proves the absence of a namespace.ID in a tree it also contains the
// leaf hashes of the range where that namespace would be.
type NamespaceRangeProof struct {
	// start index of the leaves that match the queried namespace.ID.
	start int //[start,end]
	// end index (non-inclusive) of the leaves that match the queried
	// namespace.ID.
	end int
	// Ideally preIndex == start-1 and postIndex == end+1, however,
	// there might be no value of concerned namespace.
	preIndex  int //(preIndex,postIndex)
	postIndex int

	openStart     KzgOpen
	openEnd       KzgOpen
	openPreIndex  KzgOpen
	openPostIndex KzgOpen

	// TRUE for Inclusion; FALSE for Absence
	InclusionOrAbsence bool
}

// KzgOpen, The KZG opening for a specific slot of share
type KzgOpen struct {
	index int
	value []byte
	proof KzgProof
}

func (open KzgOpen) Index() int {
	return open.index
}

func (open KzgOpen) Value() []byte {
	return open.value
}

func (open KzgOpen) Proof() KzgProof {
	return open.proof
}

func (self KzgOpen) Equal(open *KzgOpen) bool {
	return self.index == open.index &&
		bytes.Equal(self.value, open.value) &&
		bytes.Equal(self.proof, open.proof)
}

func (open KzgOpen) ToPbKzgOpen() pb.KzgOpen {
	return pb.KzgOpen{
		Index: int64(open.index),
		Value: open.value,
		Proof: open.proof,
	}
}

func NewKzgOpen(index int, value []byte, proof KzgProof) KzgOpen {
	return KzgOpen{
		index: index,
		value: value,
		proof: proof,
	}
}

func FromPbKzgOpen(open *pb.KzgOpen) KzgOpen {
	return KzgOpen{
		index: int(open.Index),
		value: open.Value,
		proof: open.Proof,
	}
}

type KzgProof []byte

// func verifyKzg(_index int, _value []byte, _commit []byte, pf KzgProof) (bool, error) {
// 	ctx, _ := gokzg4844.NewContext4096Secure()
// 	input, err := ctx.DomainByIndex(_index)
// 	if err != nil {
// 		return false, err
// 	}

// 	in := gokzg4844.Scalar{}
// 	inbytes := input.Bytes()
// 	copy(in[:], inbytes[:])

// 	clain := gokzg4844.Scalar{}
// 	copy(clain[:], _value)

// 	commit := gokzg4844.KZGCommitment{}
// 	copy(commit[:], _commit)

// 	proof := gokzg4844.KZGProof{}
// 	copy(proof[:], pf)

// 	if err := ctx.VerifyKZGProof(commit, in, clain, proof); err != nil {
// 		return false, nil
// 	}

// 	return true, nil
// }

// ToPbNamespaceRangeProof Transform from plain object into pb object
func (proof NamespaceRangeProof) ToPbNamespaceRangeProof() pb.NamespaceRangeProof {
	var (
		openStart     = proof.OpenStart().ToPbKzgOpen()
		openEnd       = proof.OpenEnd().ToPbKzgOpen()
		openPreIndex  = proof.OpenPreIndex().ToPbKzgOpen()
		openPostIndex = proof.OpenPostIndex().ToPbKzgOpen()
	)
	return pb.NamespaceRangeProof{
		Start:              int64(proof.Start()),
		End:                int64(proof.End()),
		PreIndex:           int64(proof.PreIndex()),
		PostIndex:          int64(proof.PostIndex()),
		OpenStart:          &openStart,
		OpenEnd:            &openEnd,
		OpenPreIndex:       &openPreIndex,
		OpenPostIndex:      &openPostIndex,
		InclusionOrAbsence: proof.InclusionOrAbsence,
	}
}

func (proof NamespaceRangeProof) MarshalJSON() ([]byte, error) {
	var pbProofObj = proof.ToPbNamespaceRangeProof()
	return json.Marshal(pbProofObj)
}

func (proof *NamespaceRangeProof) UnmarshalJSON(data []byte) error {
	var pbProof pb.NamespaceRangeProof
	err := json.Unmarshal(data, &pbProof)
	if err != nil {
		return err
	}
	proof.start = int(pbProof.Start)
	proof.end = int(pbProof.End)
	proof.preIndex = int(pbProof.PreIndex)
	proof.postIndex = int(pbProof.PostIndex)

	proof.openStart = FromPbKzgOpen(pbProof.OpenStart)
	proof.openEnd = FromPbKzgOpen(pbProof.OpenEnd)
	proof.openPreIndex = FromPbKzgOpen(pbProof.OpenPreIndex)
	proof.openPostIndex = FromPbKzgOpen(pbProof.OpenPostIndex)
	proof.InclusionOrAbsence = pbProof.InclusionOrAbsence
	return nil
}

func (proof NamespaceRangeProof) Equal(input *NamespaceRangeProof) bool {
	return proof.start == input.start && proof.end == input.end &&
		proof.preIndex == input.preIndex && proof.postIndex == input.postIndex &&
		proof.openStart.Equal(&input.openStart) &&
		proof.openEnd.Equal(&input.openEnd) &&
		proof.openPreIndex.Equal(&input.openPreIndex) &&
		proof.openPostIndex.Equal(&input.openPostIndex)
}

// Start index of this proof.
func (proof NamespaceRangeProof) Start() int {
	return proof.start
}

// End index of this proof, non-inclusive.
func (proof NamespaceRangeProof) End() int {
	return proof.end
}

// PreIndex pre index for the range. Normally equals to `start - 1`
func (Proof NamespaceRangeProof) PreIndex() int {
	return Proof.preIndex
}

// PostIndex post index for the range. Normally equals to `end - 1`
func (Proof NamespaceRangeProof) PostIndex() int {
	return Proof.postIndex
}

// IsNonEmptyRange returns true if this proof contains a valid, non-empty proof
// range.
func (proof NamespaceRangeProof) IsNonEmptyRange() bool {
	return proof.start >= 0 && proof.start < proof.end
}

// OpenStart
func (Proof NamespaceRangeProof) OpenStart() KzgOpen {
	return Proof.openStart
}

// OpenEnd
func (Proof NamespaceRangeProof) OpenEnd() KzgOpen {
	return Proof.openEnd
}

// OpenPreIndex
func (Proof NamespaceRangeProof) OpenPreIndex() KzgOpen {
	return Proof.openPreIndex
}

// OpenPostIndex
func (Proof NamespaceRangeProof) OpenPostIndex() KzgOpen {
	return Proof.openPostIndex
}

// TODO: Need to make compatible

func NewEmptyRangeNamespaceRangeProof(ignoreMaxNamespace bool) NamespaceRangeProof {
	var emptyKzgOpen = KzgOpen{0, nil, nil}
	return NamespaceRangeProof{0, 0, 0, 0, emptyKzgOpen, emptyKzgOpen, emptyKzgOpen, emptyKzgOpen, false}
}

func NewInclusionNamespaceRangeProof(proofStart, proofEnd int, rowCommitment []byte, ignoreMaxNamespace bool) NamespaceRangeProof {
	return NamespaceRangeProof{}
}

func NewAbsenceNamespaceRangeProof(proofStart, proofEnd int, rowCommitment []byte, leafHash []byte, ignoreMaxNamespace bool) NamespaceRangeProof {
	return NamespaceRangeProof{}
}

func (proof NamespaceRangeProof) VerifyNamespace(h hash.Hash, nID namespace.ID, commitment []byte) bool {

	nIDLen := nID.Size()
	nth := NewNmtHasher(h, nIDLen)

	// perform some consistency checks:
	// check that the root is valid w.r.t the NMT hasher
	if err := nth.ValidateCommitment(commitment); err != nil {
		return false
	}

	if proof.IsEmptyProof() && bytes.Equal(commitment, nth.EmptyCommitment()) {
		return true
	}

	if proof.IsEmptyProof() || bytes.Equal(commitment, nth.EmptyCommitment()) {
		return false
	}

	// if the proof is an absence proof, the leafHash must be valid w.r.t the NMT hasher
	if proof.IsOfAbsence() {

		return proof.VerifyAbsance(h, nID, commitment)
	}

	return proof.VerifyInclusion(h, nID, commitment)
}

// TODO: Need to make compatible
func (proof NamespaceRangeProof) VerifyInclusion(h hash.Hash, nID namespace.ID, commitment []byte) bool {
	nIDLen := nID.Size()
	nth := NewNmtHasher(h, nIDLen)

	v := proof.openStart.Value()
	if !bytes.Equal(v[:nIDLen], nID) {
		return false
	}
	if err := nth.Verify(commitment, proof.openStart); err != nil {
		return false
	}

	v = proof.openEnd.Value()
	if !bytes.Equal(v[:nIDLen], nID) {
		return false
	}
	if err := nth.Verify(commitment, proof.openEnd); err != nil {
		return false
	}

	if proof.start > 0 {
		v := proof.openPreIndex.Value()
		if bytes.Equal(v[:nIDLen], nID) {
			return false
		}
		if err := nth.Verify(commitment, proof.openPreIndex); err != nil {
			return false
		}
	}

	if proof.postIndex > 0 {
		v := proof.openPostIndex.Value()
		if bytes.Equal(v[:nIDLen], nID) {
			return false
		}
		if err := nth.Verify(commitment, proof.OpenPostIndex()); err != nil {
			return false
		}
	}

	return true
}

func (proof NamespaceRangeProof) VerifyAbsance(h hash.Hash, nID namespace.ID, commitment []byte) bool {
	nIDLen := nID.Size()
	nth := NewNmtHasher(h, nIDLen)

	if proof.preIndex != proof.postIndex-1 {
		return false
	}

	v := proof.openPreIndex.Value()
	if bytes.Equal(v[:nIDLen], nID) {
		return false
	}

	if err := nth.Verify(commitment, proof.OpenPreIndex()); err != nil {
		return false
	}

	v = proof.openPostIndex.Value()
	if bytes.Equal(v[:nIDLen], nID) {
		return false
	}
	if err := nth.Verify(commitment, proof.OpenPostIndex()); err != nil {
		return false
	}

	return true
}

func FromProtoNamespaceProof(pbProof pb.NamespaceRangeProof) NamespaceRangeProof {
	var proof NamespaceRangeProof
	proof.start = int(pbProof.Start)
	proof.end = int(pbProof.End)
	proof.preIndex = int(pbProof.PreIndex)
	proof.postIndex = int(pbProof.PostIndex)

	proof.openStart = FromPbKzgOpen(pbProof.OpenStart)
	proof.openEnd = FromPbKzgOpen(pbProof.OpenEnd)
	proof.openPreIndex = FromPbKzgOpen(pbProof.OpenPreIndex)
	proof.openPostIndex = FromPbKzgOpen(pbProof.OpenPostIndex)
	proof.InclusionOrAbsence = pbProof.InclusionOrAbsence

	return proof
}

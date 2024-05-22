package kzg

import (
	"errors"
)

var ErrFailedCompletenessCheck = errors.New("failed completeness check")

// // IsOfAbsence returns true if this proof proves the absence of leaves of a
// // namespace in the tree.
func (proof NamespaceRangeProof) IsOfAbsence() bool {
	return !proof.InclusionOrAbsence
}

// NewEmptyRangeProof constructs a proof that proves that a namespace.ID does
// not fall within the range of an NMT.
func NewEmptyRangeProof() *NamespaceRangeProof {
	return &NamespaceRangeProof{0, 0, 0, 0, KzgOpen{}, KzgOpen{}, KzgOpen{}, KzgOpen{}, false}
}

// NewInclusionProof constructs a proof that proves that a namespace.ID is
// included in an NMT.
func NewInclusionProof(proofStart, proofEnd, preIndex, postIndex int, startProof, endProof, preProof, postProof KzgOpen) *NamespaceRangeProof {
	return &NamespaceRangeProof{proofStart, proofEnd, preIndex, postIndex, startProof, endProof, preProof, postProof, true}
}

// NewAbsenceProof constructs a proof that proves that a namespace.ID falls
// within the range of an NMT but no leaf with that namespace.ID is included.
func NewAbsenceProof(pre, post int, preProof, postProof KzgOpen) *NamespaceRangeProof {
	return &NamespaceRangeProof{0, 0, pre, post, KzgOpen{}, KzgOpen{}, preProof, postProof, false}
}

func (proof NamespaceRangeProof) IsEmptyProof() bool {
	return proof.start == proof.end && proof.preIndex == proof.postIndex
}


package ipld

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"sync/atomic"

	"github.com/ipfs/boxo/blockservice"
	"github.com/ipfs/go-cid"
	ipld "github.com/ipfs/go-ipld-format"

	kzg "github.com/RiemaLabs/nubit-kzg"
	"github.com/RiemaLabs/nubit-kzg/namespace"
	share "github.com/RiemaLabs/nubit-node/da"
)

var ErrNamespaceOutsideRange = errors.New("rtrv/lp2p: " +
	"target namespace is outside of namespace range for the given root")

// Option is the functional option that is applied to the NamespaceData instance
// to configure data that needs to be stored.
type Option func(*NamespaceData)

// WithLeaves option specifies that leaves should be collected during retrieval.
func WithLeaves() Option {
	return func(data *NamespaceData) {
		// we over-allocate space for leaves since we do not know how many we will find
		// on the level above, the length of the Row is passed in as maxShares
		data.leaves = make([]ipld.Node, data.maxShares)
	}
}

func WithleafHashs() Option {
	return func(data *NamespaceData) {
		data.leafHashs = make([][]byte, data.maxShares)
	}

}

// NamespaceData stores all leaves under the given namespace with their corresponding proofs.
type NamespaceData struct {
	leaves    []ipld.Node
	leafHashs [][]byte
	//	proofs    *proofCollector

	bounds        fetchedBounds
	maxShares     int
	namespace     share.Namespace
	namespaceSize int

	isAbsentNamespace atomic.Bool
	preIndex          int
	postIndex         int

	// absenceProofLeaf ipld.Node
}

func NewNamespaceData(maxShares int, namespace share.Namespace, options ...Option) *NamespaceData {
	data := &NamespaceData{
		// we don't know where in the tree the leaves in the namespace are,
		// so we keep track of the bounds to return the correct slice
		// maxShares acts as a sentinel to know if we find any leaves
		bounds:        fetchedBounds{int64(maxShares), 0},
		maxShares:     maxShares,
		namespace:     namespace,
		namespaceSize: len(namespace),
	}

	for _, opt := range options {
		opt(data)
	}
	return data
}

func (n *NamespaceData) validate(rootCid cid.Cid) error {
	if err := n.namespace.Validate(); err != nil {
		return err
	}

	if n.leaves == nil && n.leafHashs == nil {
		return errors.New("share/ipld: empty NamespaceData, nothing specified to retrieve")
	}

	// root := NamespacedKzgFromCID(rootCid)
	// if n.namespace.IsOutsideRange(root, root) {
	// 	return ErrNamespaceOutsideRange
	// }
	return nil
}

func (n *NamespaceData) addLeaf(pos int, nd ipld.Node) {
	if bytes.Equal(n.namespace, nd.RawData()[:n.namespaceSize]) {
		n.bounds.update(int64(pos))
	}

	if n.leaves == nil {
		return
	}

	if nd != nil {
		n.leaves[pos] = nd
	}
}

// noLeaves checks that there are no leaves under the given root in the given namespace.
func (n *NamespaceData) noLeaves() bool {
	return n.bounds.lowest == int64(n.maxShares)
}

// Leaves returns retrieved leaves within the bounds in case `WithLeaves` option was passed,
// otherwise nil will be returned.
func (n *NamespaceData) Leaves() []ipld.Node {
	if n.leaves == nil || n.noLeaves() || n.isAbsentNamespace.Load() {
		return nil
	}
	return n.leaves[n.bounds.lowest : n.bounds.highest+1]
}

// Proof returns proofs within the bounds in case if `WithProofs` option was passed,
// otherwise nil will be returned.
func (n *NamespaceData) Proof() *kzg.NamespaceRangeProof {
	if n.leafHashs == nil {
		return nil
	}

	// return an empty Proof if leaves are not available
	if n.leaves == nil {
		empty := kzg.NewEmptyRangeProof()
		return empty
	}

	leavesBytes := make([][]byte, len(n.leaves))
	for i, leaf := range n.leaves {
		if leaf != nil {
			leavesBytes[i] = leaf.RawData()
		}
	}

	hasher := kzg.NewNmtHasher(sha256.New(), namespace.IDSize(n.namespaceSize))

	if n.isAbsentNamespace.Load() {
		proof, err := kzg.BuildAbsansProof(n.preIndex, n.postIndex, leavesBytes, n.leafHashs, hasher)
		if err != nil {
			log.Errorf("failed to build absence proof: %v", err)
			return nil
		}

		return proof
	}

	// build inclusion proof
	proof, err := kzg.BuildRangeProof(int(n.bounds.lowest), int(n.bounds.highest+1), leavesBytes, n.leafHashs, hasher)
	if err != nil {
		log.Errorf("failed to build inclusion proof: %v", err)
		return nil
	}
	return proof
}

// CollectLeavesByNamespace collects leaves and corresponding proof that could be used to verify
// leaves inclusion. It returns as many leaves from the given root with the given Namespace as
// it can retrieve. If no shares are found, it returns error as nil. A
// non-nil error means that only partial data is returned, because at least one share retrieval
// failed. The following implementation is based on `GetShares`.
func (n *NamespaceData) CollectLeavesByNamespace(
	ctx context.Context,
	bGetter blockservice.BlockGetter,
	root cid.Cid,
) error {
	if err := n.validate(root); err != nil {
		return err
	}

	leafHashs := GetAllLeafHash(ctx, bGetter, root, n.maxShares)
	if leafHashs == nil {
		return errors.New("rtrv/lp2p: failed to get leafHashs")
	}

	n.leafHashs = leafHashs

	return n.fetchLeaves(ctx, bGetter)
}

func (n *NamespaceData) ParseleafHashs(nd ipld.Node) {
	data := nd.RawData()
	dataSize := len(data)

	for i := 0; i < dataSize; i += NmtHashSize {
		copy(n.leafHashs[i], data[i:i+NmtHashSize])
	}
}

// func (n *NamespaceData) traverseLinks(j job, links []*ipld.Link) []job {
// 	jobs := make([]job, 0, len(links))
// 	for i, l := range links {
// 		jobs = append(jobs, job{
// 			ctx:      j.ctx,
// 			cid:      l.Cid,
// 			sharePos: i,
// 		})
// 	}
// 	return jobs
// }

type fetchedBounds struct {
	lowest  int64
	highest int64
}

// update checks if the passed index is outside the current bounds,
// and updates the bounds atomically if it extends them.
func (b *fetchedBounds) update(index int64) {
	lowest := atomic.LoadInt64(&b.lowest)
	// try to write index to the lower bound if appropriate, and retry until the atomic op is successful
	// CAS ensures that we don't overwrite if the bound has been updated in another goroutine after the
	// comparison here
	for index < lowest && !atomic.CompareAndSwapInt64(&b.lowest, lowest, index) {
		lowest = atomic.LoadInt64(&b.lowest)
	}
	// we always run both checks because element can be both the lower and higher bound
	// for example, if there is only one share in the namespace
	highest := atomic.LoadInt64(&b.highest)
	for index > highest && !atomic.CompareAndSwapInt64(&b.highest, highest, index) {
		highest = atomic.LoadInt64(&b.highest)
	}
}

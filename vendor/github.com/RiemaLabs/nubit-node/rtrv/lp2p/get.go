package ipld

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/ipfs/boxo/blockservice"
	"github.com/ipfs/go-cid"
	ipld "github.com/ipfs/go-ipld-format"

	share "github.com/RiemaLabs/nubit-node/da"
)

// NumWorkersLimit sets global limit for workers spawned by GetShares.
// GetShares could be called MaxSquareSize(128) times per data square each
// spawning up to 128/2 goroutines and altogether this is 8192. Considering
// there can be N blocks fetched at the same time, e.g. during catching up data
// from the past, we multiply this number by the amount of allowed concurrent
// data square fetches(NumConcurrentSquares).
//
// NOTE: This value only limits amount of simultaneously running workers that
// are spawned as the load increases and are killed, once the load declines.
//
// TODO(@Wondertan): This assumes we have parallelized DASer implemented. Sync the values once it is shipped.
// TODO(@Wondertan): Allow configuration of values without global state.
var NumWorkersLimit = share.MaxSquareSize * share.MaxSquareSize / 2 * NumConcurrentSquares

// NumConcurrentSquares limits the amount of squares that are fetched
// concurrently/simultaneously.
var NumConcurrentSquares = 8

// ErrNodeNotFound is used to signal when a nmt Node could not be found.
var ErrNodeNotFound = errors.New("nmt node not found")

// Global worker pool that globally controls and limits goroutines spawned by
// GetShares.
//
//	TODO(@Wondertan): Idle timeout for workers needs to be configured to around block time,
//		so that workers spawned between each reconstruction for every new block are reused.
// var pool = workerpool.New(NumWorkersLimit)

// GetLeaf fetches and returns the raw leaf.
// It walks down the IPLD NMT tree until it finds the requested one.
func GetLeaf(
	ctx context.Context,
	bGetter blockservice.BlockGetter,
	root cid.Cid,
	leaf, total int,
) (ipld.Node, error) {
	leafHashs := GetAllLeafHash(ctx, bGetter, root, total)
	if leafHashs == nil {
		return nil, ErrNodeNotFound
	}

	if len(leafHashs) < total {
		return nil, fmt.Errorf("rtrv/lp2p unexpected share size: share size %d, required index %d", len(leafHashs), total)
	}

	cid, err := CidFromNamespacedSha256(leafHashs[leaf])
	if err != nil {

		return nil, fmt.Errorf("rtrv/lp2p could not get cid from leafHash: %w", err)
	}
	return GetNode(ctx, bGetter, cid)
}

// GetLeaves gets leaves from either local storage, or, if not found, requests
// them from immediate/connected peers. It puts them into the slice under index
// of node position in the tree (bin-tree-feat).
// Does not return any error, and returns/unblocks only on success
// (got all shares) or on context cancellation.
//
// It works concurrently by spawning workers in the pool which do one basic
// thing - block until data is fetched, s. t. share processing is never
// sequential, and thus we request *all* the shares available without waiting
// for others to finish. It is the required property to maximize data
// availability. As a side effect, we get concurrent tree traversal reducing
// time to data time.
//
// GetLeaves relies on the fact that the underlying data structure is a binary
// tree, so it's not suitable for anything else besides that. Parts on the
// implementation that rely on this property are explicitly tagged with
// (bin-tree-feat).
func GetLeaves(ctx context.Context,
	bGetter blockservice.BlockGetter,
	root cid.Cid,
	maxShares int,
	put func(int, ipld.Node),
) {
	leafHashs := GetAllLeafHash(ctx, bGetter, root, maxShares)
	if leafHashs == nil {
		return
	}

	if len(leafHashs) < maxShares {
		log.Errorw("unexpected share size", "get share size", len(leafHashs), "required index", maxShares)
		return
	}

	// TODO can batch query GetBlocks
	for i, lh := range leafHashs {
		cid, err := CidFromNamespacedSha256(lh)
		if err != nil {
			log.Errorw("could not get cid from leafHash", "err", err)
			return
		}
		nd, err := GetNode(ctx, bGetter, cid)
		if err != nil {
			log.Errorw("rtrv/lp2p could not retrieve IPLD node", "pos", maxShares, "err", err)
			return
		}
		put(i, nd)
	}
}

// GetProof fetches and returns the leaf's Merkle Proof.
// It walks down the IPLD NMT tree until it reaches the leaf and returns collected proof
func GetProof(
	ctx context.Context,
	bGetter blockservice.BlockGetter,
	root cid.Cid,
	proof []cid.Cid,
	leaf, total int,
) ([]cid.Cid, error) {
	// TODO
	return nil, nil
}

// chanGroup implements an atomic wait group, closing a jobs chan
// when fully done.
// type chanGroup struct {
// 	jobs    chan job
// 	counter int64
// }

// func (w *chanGroup) add(count int64) {
// 	atomic.AddInt64(&w.counter, count)
// }

// func (w *chanGroup) done() {
// 	numRemaining := atomic.AddInt64(&w.counter, -1)

// 	// Close channel if this job was the last one
// 	if numRemaining == 0 {
// 		close(w.jobs)
// 	}
// }

// job represents an encountered node to investigate during the `GetLeaves`
// and `CollectLeavesByNamespace` routines.
// type job struct {
// 	// we pass the context to job so that spans are tracked in a tree
// 	// structure
// 	ctx context.Context
// 	// cid of the node that will be handled
// 	cid cid.Cid
// 	// sharePos represents potential share position in share slice
// 	sharePos int
// 	// depth represents the number of edges present in path from the root node of a tree to that node
// 	//depth int
// 	// isAbsent indicates if target namespaceID is not included, only collect absence proofs
// 	//isAbsent bool
// }

func GetAllLeafHash(ctx context.Context, bGetter blockservice.BlockGetter, root cid.Cid, maxShare int) [][]byte {
	nd, err := GetNode(ctx, bGetter, root)
	if err != nil {
		log.Errorw("rtrv/lp2p: could not retrieve all leafHashs node",
			"root", root.String(),
			"err", err,
		)
		return nil
	}

	body := nd.RawData()
	if len(body) != maxShare*NmtHashSize {
		log.Errorw("rtrv/lp2p: get unexpected leafhashs data", "expect size", maxShare*NmtHashSize, "actual size", len(body))
		return nil
	}

	leafHashs := make([][]byte, maxShare)

	for i := 0; i < maxShare; i++ {
		leafHashs[i] = body[i*NmtHashSize : (i+1)*NmtHashSize]
	}
	return leafHashs
}

func (n *NamespaceData) fetchLeaves(ctx context.Context, bGetter blockservice.BlockGetter) error {
	find, index, err := n.binarySearch(ctx, bGetter)
	if err != nil {
		return err
	}

	// need generate inclusion proof
	if find {
		var retriveErr atomic.Value
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		left := index
		right := index

		wg := sync.WaitGroup{}
		wg.Add(2)
		go func() {
			defer wg.Done()
			for left > 0 {
				select {
				case <-ctx.Done():
					return
				default:
				}

				left--
				cid := MustCidFromNamespacedSha256(n.leafHashs[left])
				nd, err := GetNode(ctx, bGetter, cid)
				if err != nil {
					log.Errorw("rtrv/lp2p could not retrieve IPLD node",
						"namespace", n.namespace.String(),
						"pos", left,
						"err", err,
					)
					retriveErr.Store(err)
					cancel()
					return
				}
				n.addLeaf(left, nd)
				if !bytes.Equal(nd.RawData()[:n.namespaceSize], n.namespace) {
					return
				}
			}
		}()

		go func() {
			wg.Done()
			for right < n.maxShares-1 {
				select {
				case <-ctx.Done():
					return
				default:
				}

				right++
				cid := MustCidFromNamespacedSha256(n.leafHashs[right])
				nd, err := GetNode(ctx, bGetter, cid)
				if err != nil {
					log.Errorw("could not retrieve IPLD node",
						"namespace", n.namespace.String(),
						"pos", right,
						"err", err,
					)
					retriveErr.Store(err)
					cancel()
					return
				}
				n.addLeaf(right, nd)
				if !bytes.Equal(nd.RawData()[:n.namespaceSize], n.namespace) {
					return
				}

			}
		}()
		wg.Wait()
		if err, ok := retriveErr.Load().(error); ok {
			return err
		}
	}

	return nil
}

func (n *NamespaceData) binarySearch(ctx context.Context, bGetter blockservice.BlockGetter) (bool, int, error) {
	left := 0
	right := n.maxShares - 1

	for left <= right {
		mid := (right-left)/2 + left
		cid := MustCidFromNamespacedSha256(n.leafHashs[mid])
		nd, err := GetNode(ctx, bGetter, cid)
		if err != nil {
			log.Errorw("could not retrieve IPLD node",
				"namespace", n.namespace.String(),
				"pos", mid,
				"err", err,
			)
			return false, -1, err
		}
		n.addLeaf(mid, nd)
		if n.namespace.Equals(nd.RawData()[:n.namespaceSize]) {
			return true, mid, nil
		}

		if n.namespace.IsGreater(nd.RawData()[:n.namespaceSize]) {
			left = mid + 1
		} else {
			right = mid - 1
		}
	}

	n.isAbsentNamespace.Store(true)
	n.preIndex = right
	n.postIndex = left

	return false, -1, nil
}

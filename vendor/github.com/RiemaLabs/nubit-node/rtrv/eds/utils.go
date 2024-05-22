package eds

import (
	"context"
	"errors"
	"fmt"
	"io"
	"runtime"

	"github.com/dgraph-io/badger/v4/options"
	"github.com/filecoin-project/dagstore/index"
	"github.com/filecoin-project/dagstore/shard"
	ds "github.com/ipfs/go-datastore"
	dsbadger "github.com/ipfs/go-ds-badger4"
	"github.com/multiformats/go-multihash"

	"github.com/filecoin-project/dagstore"
	"github.com/ipfs/boxo/blockservice"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/sync/errgroup"

	share "github.com/RiemaLabs/nubit-node/da"
	"github.com/RiemaLabs/nubit-node/rtrv/eds/cache"
	"github.com/RiemaLabs/nubit-node/rtrv/lp2p"
	"github.com/RiemaLabs/nubit-node/strucs/utils/utils"
)

// readCloser is a helper struct, that combines io.Reader and io.Closer
type readCloser struct {
	io.Reader
	io.Closer
}

// BlockstoreCloser represents a blockstore that can also be closed. It combines the functionality
// of a dagstore.ReadBlockstore with that of an io.Closer.
type BlockstoreCloser struct {
	dagstore.ReadBlockstore
	io.Closer
}

func newReadCloser(ac cache.Accessor) io.ReadCloser {
	return readCloser{
		ac.Reader(),
		ac,
	}
}

// blockstoreCloser constructs new BlockstoreCloser from cache.Accessor
func blockstoreCloser(ac cache.Accessor) (*BlockstoreCloser, error) {
	bs, err := ac.Blockstore()
	if err != nil {
		return nil, fmt.Errorf("eds/store: failed to get blockstore: %w", err)
	}
	return &BlockstoreCloser{
		ReadBlockstore: bs,
		Closer:         ac,
	}, nil
}

func closeAndLog(name string, closer io.Closer) {
	if err := closer.Close(); err != nil {
		log.Warnw("closing "+name, "err", err)
	}
}

// RetrieveNamespaceFromStore gets all EDS shares in the given namespace from
// the EDS store through the corresponding CAR-level blockstore. It is extracted
// from the store getter to make it available for reuse in the p2pnd server.
func RetrieveNamespaceFromStore(
	ctx context.Context,
	store *Store,
	dah *share.Root,
	namespace share.Namespace,
) (shares share.NamespacedShares, err error) {
	if err = namespace.ValidateForData(); err != nil {
		return nil, err
	}

	bs, err := store.CARBlockstore(ctx, dah.Hash())
	if errors.Is(err, ErrNotFound) {
		// convert error to satisfy getter interface contract
		err = share.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve blockstore from eds store: %w", err)
	}
	defer func() {
		if err := bs.Close(); err != nil {
			log.Warnw("closing blockstore", "err", err)
		}
	}()

	// wrap the read-only CAR blockstore in a getter
	blockGetter := NewBlockGetter(bs)
	shares, err = CollectSharesByNamespace(ctx, blockGetter, dah, namespace)
	if errors.Is(err, ipld.ErrNodeNotFound) {
		// IPLD node not found after the index pointed to this shard and the CAR
		// blockstore has been opened successfully is a strong indicator of
		// corruption. We remove the block on bridges and fulls and return
		// share.ErrNotFound to ensure the data is retrieved by the next getter.
		// Note that this recovery is manual and will only be restored by an RPC
		// call to SharesAvailable that fetches the same datahash that was
		// removed.
		err = store.Remove(ctx, dah.Hash())
		if err != nil {
			log.Errorf("failed to remove CAR from store after detected corruption: %w", err)
		}
		err = share.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve shares by namespace from store: %w", err)
	}

	return shares, nil
}

// CollectSharesByNamespace collects NamespaceShares within the given namespace from share.Root.
func CollectSharesByNamespace(
	ctx context.Context,
	bg blockservice.BlockGetter,
	root *share.Root,
	namespace share.Namespace,
) (shares share.NamespacedShares, err error) {
	ctx, span := tracer.Start(ctx, "collect-shares-by-namespace", trace.WithAttributes(
		attribute.String("namespace", namespace.String()),
	))
	defer func() {
		utils.SetStatusAndEnd(span, err)
	}()

	rootCIDs := ipld.FilterRootByNamespace(root, namespace)
	if len(rootCIDs) == 0 {
		return []share.NamespacedRow{}, nil
	}

	errGroup, ctx := errgroup.WithContext(ctx)
	shares = make([]share.NamespacedRow, len(rootCIDs))
	for i, rootCID := range rootCIDs {
		// shadow loop variables, to ensure correct values are captured
		i, rootCID := i, rootCID
		errGroup.Go(func() error {
			row, proof, err := ipld.GetSharesByNamespace(ctx, bg, rootCID, namespace, len(root.RowRoots))
			shares[i] = share.NamespacedRow{
				Shares: row,
				Proof:  proof,
			}
			if err != nil {
				return fmt.Errorf("retrieving shares by namespace %s for row %x: %w", namespace.String(), rootCID, err)
			}
			return nil
		})
	}

	if err := errGroup.Wait(); err != nil {
		return nil, err
	}

	return shares, nil
}

const invertedIndexPath = "/inverted_index/"

// ErrNotFoundInIndex is returned instead of ErrNotFound if the multihash doesn't exist in the index
var ErrNotFoundInIndex = errors.New("does not exist in index")

// simpleInvertedIndex is an inverted index that only stores a single shard key per multihash. Its
// implementation is modified from the default upstream implementation in dagstore/index.
type simpleInvertedIndex struct {
	ds ds.Batching
}

// newSimpleInvertedIndex returns a new inverted index that only stores a single shard key per
// multihash. This is because we use badger as a storage backend, so updates are expensive, and we
// don't care which shard is used to serve a cid.
func newSimpleInvertedIndex(storePath string) (*simpleInvertedIndex, error) {
	opts := dsbadger.DefaultOptions // this should be copied
	// turn off value log GC as we don't use value log
	opts.GcInterval = 0
	// use minimum amount of NumLevelZeroTables to trigger L0 compaction faster
	opts.NumLevelZeroTables = 1
	// MaxLevels = 8 will allow the db to grow to ~11.1 TiB
	opts.MaxLevels = 8
	// inverted index stores unique hash keys, so we don't need to detect conflicts
	opts.DetectConflicts = false
	// we don't need compression for inverted index as it just hashes
	opts.Compression = options.None
	compactors := runtime.NumCPU()
	if compactors < 2 {
		compactors = 2
	}
	if compactors > opts.MaxLevels { // ensure there is no more compactors than db table levels
		compactors = opts.MaxLevels
	}
	opts.NumCompactors = compactors

	ds, err := dsbadger.NewDatastore(storePath+invertedIndexPath, &opts)
	if err != nil {
		return nil, fmt.Errorf("can't open Badger Datastore: %w", err)
	}

	return &simpleInvertedIndex{ds: ds}, nil
}

func (s *simpleInvertedIndex) AddMultihashesForShard(
	ctx context.Context,
	mhIter index.MultihashIterator,
	sk shard.Key,
) error {
	// in the original implementation, a mutex is used here to prevent unnecessary updates to the
	// key. The amount of extra data produced by this is negligible, and the performance benefits
	// from removing the lock are significant (indexing is a hot path during sync).
	batch, err := s.ds.Batch(ctx)
	if err != nil {
		return fmt.Errorf("failed to create ds batch: %w", err)
	}

	err = mhIter.ForEach(func(mh multihash.Multihash) error {
		key := ds.NewKey(string(mh))
		if err := batch.Put(ctx, key, []byte(sk.String())); err != nil {
			return fmt.Errorf("failed to put mh=%s, err=%w", mh, err)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to add index entry: %w", err)
	}

	if err := batch.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit batch: %w", err)
	}
	return nil
}

func (s *simpleInvertedIndex) GetShardsForMultihash(ctx context.Context, mh multihash.Multihash) ([]shard.Key, error) {
	key := ds.NewKey(string(mh))
	sbz, err := s.ds.Get(ctx, key)
	if err != nil {
		return nil, errors.Join(ErrNotFoundInIndex, err)
	}

	return []shard.Key{shard.KeyFromString(string(sbz))}, nil
}

func (s *simpleInvertedIndex) close() error {
	return s.ds.Close()
}

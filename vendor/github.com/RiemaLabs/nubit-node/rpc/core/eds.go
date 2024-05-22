package core

import (
	"context"
	"errors"
	"fmt"

	"github.com/filecoin-project/dagstore"
	"github.com/tendermint/tendermint/types"

	"github.com/RiemaLabs/nubit-validator/tx"
	"github.com/RiemaLabs/nubit-validator/utils/appconsts"
	"github.com/RiemaLabs/nubit-validator/da/shares"
	"github.com/RiemaLabs/nubit-validator/da/square"
	"github.com/RiemaLabs/nubit-validator/utils/wrapper"
	kzg "github.com/RiemaLabs/nubit-kzg"
	"github.com/RiemaLabs/rsmt2d"

	"github.com/RiemaLabs/nubit-node/da"
	"github.com/RiemaLabs/nubit-node/rtrv/eds"
)

// extendBlock extends the given block data, returning the resulting
// ExtendedDataSquare (EDS). If there are no transactions in the block,
// nil is returned in place of the eds.
func extendBlock(data types.Data, appVersion uint64, options ...kzg.Option) (*rsmt2d.ExtendedDataSquare, error) {
	if app.IsEmptyBlock(data, appVersion) {
		return nil, nil
	}

	// Construct the data square from the block's transactions
	dataSquare, err := square.Construct(data.Txs.ToSliceOfBytes(), appVersion, appconsts.SquareSizeUpperBound(appVersion))
	if err != nil {
		return nil, err
	}
	return extendShares(shares.ToBytes(dataSquare), options...)
}

func extendShares(s [][]byte, options ...kzg.Option) (*rsmt2d.ExtendedDataSquare, error) {
	// Check that the length of the square is a power of 2.
	if !shares.IsPowerOfTwo(len(s)) {
		return nil, fmt.Errorf("number of shares is not a power of 2: got %d", len(s))
	}
	// here we construct a tree
	// Note: uses the nmt wrapper to construct the tree.
	squareSize := square.Size(len(s))
	return rsmt2d.ComputeExtendedDataSquare(s,
		appconsts.DefaultCodec(),
		wrapper.NewConstructor(uint64(squareSize),
			options...))
}

// storeEDS will only store extended block if it is not empty and doesn't already exist.
func storeEDS(ctx context.Context, hash share.DataHash, eds *rsmt2d.ExtendedDataSquare, store *eds.Store) error {
	if eds == nil {
		return nil
	}
	err := store.Put(ctx, hash, eds)
	if errors.Is(err, dagstore.ErrShardExists) {
		// block with given root already exists, return nil
		return nil
	}
	return err
}

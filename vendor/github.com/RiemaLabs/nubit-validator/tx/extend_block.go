package app

import (
	"github.com/RiemaLabs/nubit-validator/da/da"
	"github.com/RiemaLabs/nubit-validator/da/shares"
	"github.com/RiemaLabs/nubit-validator/da/square"
	"github.com/RiemaLabs/nubit-validator/utils/appconsts"
	"github.com/RiemaLabs/rsmt2d"
	sdk "github.com/cosmos/cosmos-sdk/types"
	coretypes "github.com/tendermint/tendermint/types"
)

// GovSquareSizeUpperBound returns the maximum square size that can be used for a block
// using the governance parameter blob.GovMaxSquareSize.
func (app *App) GovSquareSizeUpperBound(ctx sdk.Context) int {
	// TODO: fix hack that forces the max square size for the first height to
	// 64. This is due to our fork of the sdk not initializing state before
	// BeginBlock of the first block. This is remedied in versions of the sdk
	// and comet that have full support of PreparePropsoal, although
	// nubit-validator does not currently use those. see this PR for more details
	// https://github.com/cosmos/cosmos-sdk/pull/14505
	if ctx.BlockHeader().Height <= 1 {
		return int(appconsts.DefaultGovMaxSquareSize)
	}

	gmax := int(app.BlobKeeper.GovMaxSquareSize(ctx))
	// perform a secondary check on the max square size.
	if gmax > appconsts.SquareSizeUpperBound(app.AppVersion(ctx)) {
		gmax = appconsts.SquareSizeUpperBound(app.AppVersion(ctx))
	}

	return gmax
}

// ExtendBlock extends the given block data into a data square for a given app
// version.
func ExtendBlock(data coretypes.Data, appVersion uint64) (*rsmt2d.ExtendedDataSquare, error) {
	// Construct the data square from the block's transactions
	dataSquare, err := square.Construct(data.Txs.ToSliceOfBytes(), appVersion, appconsts.SquareSizeUpperBound(appVersion))
	if err != nil {
		return nil, err
	}

	return da.ExtendShares(shares.ToBytes(dataSquare))
}

// EmptyBlock returns true if the given block data is considered empty by the
// application at a given version.
func IsEmptyBlock(data coretypes.Data, _ uint64) bool {
	return len(data.Txs) == 0
}

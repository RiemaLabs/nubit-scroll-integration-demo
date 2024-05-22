package app

import (
	"time"

	"github.com/RiemaLabs/nubit-validator/auxiliary/ante"
	"github.com/RiemaLabs/nubit-validator/da/da"
	"github.com/RiemaLabs/nubit-validator/da/shares"
	"github.com/RiemaLabs/nubit-validator/da/square"
	"github.com/cosmos/cosmos-sdk/telemetry"
	abci "github.com/tendermint/tendermint/abci/types"
	core "github.com/tendermint/tendermint/proto/tendermint/types"
)

// PrepareProposal fulfills the nubit-core version of the ABCI interface by
// preparing the proposal block data. The square size is determined by first
// estimating it via the size of the passed block data. Then, this method
// generates the data root for the proposal block and passes it back to
// tendermint via the BlockData. Panics indicate a developer error and should
// immediately halt the node for visibility and so they can be quickly resolved.
func (app *App) PrepareProposal(req abci.RequestPrepareProposal) abci.ResponsePrepareProposal {
	defer telemetry.MeasureSince(time.Now(), "prepare_proposal")
	// create a context using a branch of the state and loaded using the
	// proposal height and chain-id
	sdkCtx := app.NewProposalContext(core.Header{
		ChainID: req.ChainId,
		Height:  req.Height,
		Time:    req.Time,
	})
	// filter out invalid transactions.
	// TODO: we can remove all state independent checks from the ante handler here such as signature verification
	// and only check the state dependent checks like fees and nonces as all these transactions have already
	// passed CheckTx.
	handler := ante.NewAnteHandler(
		app.AccountKeeper,
		app.BankKeeper,
		app.BlobKeeper,
		app.FeeGrantKeeper,
		app.GetTxConfig().SignModeHandler(),
		ante.DefaultSigVerificationGasConsumer,
		app.IBCKeeper,
	)

	app.Logger().Info("app: start prepare proposal", "height", req.Height, "numTxs", len(req.BlockData.Txs))
	var txs [][]byte
	// This if statement verifies whether the preparation of the proposal
	// pertains to the first block. If it does, the block is constructed using
	// an empty set of transactions. However, even without this validation,
	// the initial block is anticipated to be devoid of transactions, as
	// established by the findings presented in
	// https://github.com/RiemaLabs/nubit-validator/issues/1899;
	// The inclusion of this check is out of an abundance of caution.
	// The rationale behind having an empty first block revolves around the fact
	// that no transactions can enter the mempool since no committed state exists
	// until after the first block is committed (at which point the Genesis state
	// gets committed too). Consequently, the prepare proposal request for the
	// first block is expected to contain no transaction, so is the first block.
	if app.LastBlockHeight() == 0 {
		txs = make([][]byte, 0)
		if len(req.BlockData.Txs) != 0 {
			// if the consensus layer sends non-empty set of transactions for
			// block height 1, log it
			app.Logger().Info(
				"non-empty txs received from the consensus layer for block height 1",
				"numberOfTransactions",
				len(req.BlockData.Txs),
			)
		}
	} else {
		txs = FilterTxs(app.Logger(), sdkCtx, handler, app.txConfig, req.BlockData.Txs)
	}

	// build the square from the set of valid and prioritised transactions.
	// The txs returned are the ones used in the square and block
	//btcHeight, err := FetchHeight(app.BTCRpc)
	//if err != nil {
	//	app.Logger().Error("app: btc height fetch failure", "err", err)
	//	btcHeight = app.LatestBtcHeight
	//} else {
	//	app.LatestBtcHeight = btcHeight
	//	if app.LatestBtcHeightNotRecorded {
	//		app.LatestBtcHeightNotRecorded = false
	//	}
	//}

	//txs, err = ExtendTxsWithBtcRef(btcHeight, txs)
	//if err != nil {
	//	app.Logger().Error("app: failure to extend txs with btc height", "error", err.Error())
	//	panic(err)
	//}
	dataSquare, txs, err := square.Build(txs, app.GetBaseApp().AppVersion(sdkCtx), app.GovSquareSizeUpperBound(sdkCtx))
	if err != nil {
		app.Logger().Error("app: failure to build the data square while creating a proposal block", "error", err.Error())
		panic(err)
	}

	// erasure the data square which we use to create the data root.
	// Note: uses the nmt wrapper to construct the tree.
	// checkout pkg/wrapper/nmt_wrapper.go for more information.
	eds, err := da.ExtendShares(shares.ToBytes(dataSquare))
	if err != nil {
		app.Logger().Error(
			"app: failure to erasure the data square while creating a proposal block",
			"error",
			err.Error(),
		)
		panic(err)
	}

	// create the new data root by creating the data availability header (merkle
	// roots of each row and col of the erasure data).
	dah, err := da.NewDataAvailabilityHeader(eds)
	if err != nil {
		app.Logger().Error(
			"failure to create new data availability header",
			"error",
			err.Error(),
		)
		panic(err)
	}
	//btcHeight, err = GetBtcHeightFromTx(eds.GetBtcHeightTx())
	//if err != nil {
	//	app.Logger().Error("app: failure to get btc height from tx", "error", err.Error())
	//	panic(err)
	//}
	//app.Logger().Info("app: prepare proposal", "height", req.Height,
	//	"DataHash", fmt.Sprintf("%X", dah.Hash()), "numTxs", len(txs), "btcHeight", btcHeight)

	// tendermint doesn't need to use any of the erasure data, as only the
	// protobuf encoded version of the block data is gossiped.
	return abci.ResponsePrepareProposal{
		BlockData: &core.Data{
			Txs:        txs,
			SquareSize: uint64(dataSquare.Size()),
			Hash:       dah.Hash(),
		},
	}
}

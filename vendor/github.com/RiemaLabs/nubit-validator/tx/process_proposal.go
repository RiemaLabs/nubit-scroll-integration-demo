package app

import (
	"bytes"
	"fmt"
	"time"

	"github.com/RiemaLabs/nubit-validator/auxiliary/ante"
	"github.com/RiemaLabs/nubit-validator/da/da"
	"github.com/RiemaLabs/nubit-validator/da/shares"
	"github.com/RiemaLabs/nubit-validator/da/square"
	blobtypes "github.com/RiemaLabs/nubit-validator/utils/tk/sh/blob/types"
	"github.com/cosmos/cosmos-sdk/telemetry"
	sdk "github.com/cosmos/cosmos-sdk/types"
	abci "github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/libs/log"
	tmproto "github.com/tendermint/tendermint/proto/tendermint/types"
	coretypes "github.com/tendermint/tendermint/types"
)

const rejectedPropBlockLog = "Rejected proposal block:"

func (app *App) ProcessProposal(req abci.RequestProcessProposal) (resp abci.ResponseProcessProposal) {
	defer telemetry.MeasureSince(time.Now(), "process_proposal")
	// In the case of a panic from an unexpected condition, it is better for the liveness of the
	// network that we catch it, log an error and vote nil than to crash the node.
	defer func() {
		if err := recover(); err != nil {
			logInvalidPropBlock(app.Logger(), req.Header, fmt.Sprintf("caught panic: %v", err))
			telemetry.IncrCounter(1, "process_proposal", "panics")
			resp = reject()
		}
	}()
	app.Logger().Info("app: start Process Proposal", "height", req.Header.Height, "proposer", fmt.Sprintf("%X", req.Header.ProposerAddress))

	// Create the anteHander that are used to check the validity of
	// transactions. All transactions need to be equally validated here
	// so that the nonce number is always correctly incremented (which
	// may affect the validity of future transactions).
	handler := ante.NewAnteHandler(
		app.AccountKeeper,
		app.BankKeeper,
		app.BlobKeeper,
		app.FeeGrantKeeper,
		app.GetTxConfig().SignModeHandler(),
		ante.DefaultSigVerificationGasConsumer,
		app.IBCKeeper,
	)
	sdkCtx := app.NewProposalContext(req.Header)

	// iterate over all txs and ensure that all blobTxs are valid, PFBs are correctly signed and non
	// blobTxs have no PFBs present
	txs := req.BlockData.Txs
	if len(txs) > 0 {
		txs = req.BlockData.Txs[1:]
	}
	for idx, rawTx := range txs {
		tx := rawTx
		blobTx, isBlobTx := coretypes.UnmarshalBlobTx(rawTx)
		if isBlobTx {
			tx = blobTx.Tx
		}

		sdkTx, err := app.txConfig.TxDecoder()(tx)
		if err != nil {
			// we don't reject the block here because it is not a block validity
			// rule that all transactions included in the block data are
			// decodable
			continue
		}

		// handle non-blob transactions first
		if !isBlobTx {
			_, has := hasPFB(sdkTx.GetMsgs())
			if has {
				// A non blob tx has a PFB, which is invalid
				logInvalidPropBlock(app.Logger(), req.Header, fmt.Sprintf("tx %d has PFB but is not a blob tx", idx))
				return reject()
			}

			// we need to increment the sequence for every transaction so that
			// the signature check below is accurate. this error only gets hit
			// if the account in question doens't exist.
			sdkCtx, err = handler(sdkCtx, sdkTx, false)
			if err != nil {
				logInvalidPropBlockError(app.Logger(), req.Header, "failure to incrememnt sequence", err)
				return reject()
			}

			// we do not need to perform further checks on this transaction,
			// since it has no PFB
			continue
		}

		// validate the blobTx. This is the same validation used in CheckTx ensuring
		// - there is one PFB
		// - that each blob has a valid namespace
		// - that the sizes match
		// - that the namespaces match between blob and PFB
		// - that the share commitment is correct
		if err := blobtypes.ValidateBlobTx(app.txConfig, blobTx); err != nil {
			logInvalidPropBlockError(app.Logger(), req.Header, fmt.Sprintf("invalid blob tx %d", idx), err)
			return reject()
		}

		// validated the PFB signature
		sdkCtx, err = handler(sdkCtx, sdkTx, false)
		if err != nil {
			logInvalidPropBlockError(app.Logger(), req.Header, "invalid PFB signature", err)
			return reject()
		}

	}

	// Construct the data square from the block's transactions
	dataSquare, err := square.Construct(req.BlockData.Txs, app.GetBaseApp().AppVersion(sdkCtx), app.GovSquareSizeUpperBound(sdkCtx))
	if err != nil {
		logInvalidPropBlockError(app.Logger(), req.Header, "failure to compute data square from transactions:", err)
		return reject()
	}

	// Assert that the square size stated by the proposer is correct
	if uint64(dataSquare.Size()) != req.BlockData.SquareSize {
		logInvalidPropBlock(app.Logger(), req.Header, "proposed square size differs from calculated square size")
		return reject()
	}

	eds, err := da.ExtendShares(shares.ToBytes(dataSquare))
	if err != nil {
		logInvalidPropBlockError(app.Logger(), req.Header, "failure to erasure the data square", err)
		return reject()
	}

	dah, err := da.NewDataAvailabilityHeader(eds)
	if err != nil {
		logInvalidPropBlockError(app.Logger(), req.Header, "failure to create new data availability header", err)
		return reject()
	}
	// by comparing the hashes we know the computed IndexWrappers (with the share indexes of the PFB's blobs)
	// are identical and that square layout is consistent. This also means that the share commitment rules
	// have been followed and thus each blobs share commitment should be valid
	if !bytes.Equal(dah.Hash(), req.Header.DataHash) {
		logInvalidPropBlock(app.Logger(), req.Header, fmt.Sprintf("proposed data root %X differs from calculated data root %X", req.Header.DataHash, dah.Hash()))
		return reject()
	}
	app.Logger().Debug("app: process proposal", "height", req.Header.Height, "dataRoot", dah.Hash(), "numTxs", len(req.BlockData.Txs))

	if req.IsSync {
		return accept()
	}

	//btcHeight, err := GetBtcHeightFromTx(eds.GetBtcHeightTx())
	//if err != nil {
	//	app.Logger().Error("app: failure to get btc height from tx", "error", err.Error())
	//	panic(err)
	//}
	//refRes, err := CheckTxsWithBtcRef(btcHeight, app.LatestBtcHeight, app.LatestBtcHeightNotRecorded, app.BTCRpc)
	//if err != nil {
	//	logInvalidPropBlockError(app.Logger(), req.Header, "failure to execute CheckTxsWithBtcRef", err)
	//	return reject()
	//}
	//if !refRes {
	//	logInvalidPropBlockError(app.Logger(), req.Header, "invalid btcRef", err)
	//	return reject()
	//}
	//if app.LatestBtcHeightNotRecorded {
	//	app.LatestBtcHeightNotRecorded = false
	//}
	//app.LatestBtcHeight = binary.BigEndian.Uint64(req.BlockData.Txs[0])
	//app.LatestBtcHeight, _ = GetBtcHeightFromTx(req.BlockData.Txs[0])

	return accept()
}

func hasPFB(msgs []sdk.Msg) (*blobtypes.MsgSubmitBlobPayments, bool) {
	for _, msg := range msgs {
		if pfb, ok := msg.(*blobtypes.MsgSubmitBlobPayments); ok {
			return pfb, true
		}
	}
	return nil, false
}

func logInvalidPropBlock(l log.Logger, h tmproto.Header, reason string) {
	l.Error(
		rejectedPropBlockLog,
		"reason",
		reason,
		"proposer",
		h.ProposerAddress,
	)
}

func logInvalidPropBlockError(l log.Logger, h tmproto.Header, reason string, err error) {
	l.Error(
		rejectedPropBlockLog,
		"reason",
		reason,
		"proposer",
		h.ProposerAddress,
		"err",
		err.Error(),
	)
}

func reject() abci.ResponseProcessProposal {
	return abci.ResponseProcessProposal{
		Result: abci.ResponseProcessProposal_REJECT,
	}
}

func accept() abci.ResponseProcessProposal {
	return abci.ResponseProcessProposal{
		Result: abci.ResponseProcessProposal_ACCEPT,
	}
}

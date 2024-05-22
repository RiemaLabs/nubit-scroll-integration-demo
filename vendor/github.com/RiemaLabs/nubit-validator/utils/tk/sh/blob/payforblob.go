package blob

import (
	"context"

	"google.golang.org/grpc"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdktx "github.com/cosmos/cosmos-sdk/types/tx"

	"github.com/RiemaLabs/nubit-validator/utils/tk/sh/blob/types"
	coretypes "github.com/tendermint/tendermint/types"
)

// SubmitBlobs builds, signs, and synchronously submits a SubmitBlobPayment
// transaction. It returns a sdk.TxResponse after submission.
func SubmitBlob(
	ctx context.Context,
	signer *types.KeyringSigner,
	conn *grpc.ClientConn,
	mode sdktx.BroadcastMode,
	blobs []*types.Blob,
	opts ...types.TxBuilderOption,
) (*sdk.TxResponse, error) {
	addr, err := signer.GetSignerInfo().GetAddress()
	if err != nil {
		return nil, err
	}
	msg, err := types.NewMsgSubmitBlobPayments(addr.String(), blobs...)
	if err != nil {
		return nil, err
	}
	err = signer.QueryAccountNumber(ctx, conn)
	if err != nil {
		return nil, err
	}
	builder := signer.NewTxBuilder(opts...)
	stx, err := signer.BuildSignedTx(builder, msg)
	if err != nil {
		return nil, err
	}
	rawTx, err := signer.EncodeTx(stx)
	if err != nil {
		return nil, err
	}
	blobTx, err := coretypes.MarshalBlobTx(rawTx, blobs...)
	if err != nil {
		return nil, err
	}
	txResp, err := types.BroadcastTx(ctx, conn, mode, blobTx)
	if err != nil {
		return nil, err
	}
	return txResp.TxResponse, nil
}

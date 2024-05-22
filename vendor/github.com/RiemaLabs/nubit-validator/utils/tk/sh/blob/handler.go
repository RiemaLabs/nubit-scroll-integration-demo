package blob

import (
	"fmt"

	"cosmossdk.io/errors"
	"github.com/RiemaLabs/nubit-validator/utils/tk/sh/blob/keeper"
	"github.com/RiemaLabs/nubit-validator/utils/tk/sh/blob/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
)

// NewHandler uses the provided blob keeper to create an sdk.Handler
func NewHandler(k keeper.Keeper) sdk.Handler {
	msgServer := keeper.NewMsgServerImpl(k)

	return func(ctx sdk.Context, msg sdk.Msg) (*sdk.Result, error) {
		ctx = ctx.WithEventManager(sdk.NewEventManager())
		switch msg := msg.(type) {
		case *types.MsgSubmitBlobPayments:
			res, err := msgServer.SubmitBlobPayments(sdk.WrapSDKContext(ctx), msg)
			return sdk.WrapServiceResult(ctx, res, err)
		default:
			errMsg := fmt.Sprintf("unrecognized %s message type: %T", types.ModuleName, msg)
			return nil, errors.Wrap(sdkerrors.ErrUnknownRequest, errMsg)
		}
	}
}

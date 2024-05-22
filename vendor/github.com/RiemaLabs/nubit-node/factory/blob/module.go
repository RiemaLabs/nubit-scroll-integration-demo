package blob

import (
	"context"

	"go.uber.org/fx"

	"github.com/RiemaLabs/nubit-node/strucs/btx"
	"github.com/RiemaLabs/nubit-node/strucs/eh"
	headerService "github.com/RiemaLabs/nubit-node/factory/header"
	"github.com/RiemaLabs/nubit-node/factory/state"
	"github.com/RiemaLabs/nubit-node/da"
)

func ConstructModule() fx.Option {
	return fx.Module("blob",
		fx.Provide(
			func(service headerService.Module) func(context.Context, uint64) (*header.ExtendedHeader, error) {
				return service.GetByHeight
			}),
		fx.Provide(func(
			state state.Module,
			sGetter share.Getter,
			getByHeightFn func(context.Context, uint64) (*header.ExtendedHeader, error),
		) Module {
			return blob.NewService(state, sGetter, getByHeightFn)
		}))
}

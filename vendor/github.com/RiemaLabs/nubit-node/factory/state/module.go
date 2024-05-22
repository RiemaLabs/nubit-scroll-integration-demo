package state

import (
	"context"

	logging "github.com/ipfs/go-log/v2"
	"go.uber.org/fx"

	"github.com/RiemaLabs/nubit-node/factory/core"
	"github.com/RiemaLabs/nubit-node/factory/node"
	"github.com/RiemaLabs/nubit-node/strucs/state"
	"github.com/RiemaLabs/nubit-node/strucs/utils/fxutil"
)

var log = logging.Logger("module/state")

// ConstructModule provides all components necessary to construct the
// state service.
func ConstructModule(tp node.Type, cfg *Config, coreCfg *core.Config) fx.Option {
	// sanitize config values before constructing module
	cfgErr := cfg.Validate()

	baseComponents := fx.Options(
		fx.Supply(*cfg),
		fx.Error(cfgErr),
		fxutil.ProvideIf(coreCfg.IsEndpointConfigured(), fx.Annotate(
			coreAccessor,
			fx.OnStart(func(ctx context.Context,
				breaker *state.CoreAccessor) error {
				return breaker.Start(ctx)
			}),
			fx.OnStop(func(ctx context.Context,
				breaker *state.CoreAccessor) error {
				return breaker.Stop(ctx)
			}),
		)),
		fxutil.ProvideIf(!coreCfg.IsEndpointConfigured(), func() (*state.CoreAccessor, Module) {
			return nil, &stubbedStateModule{}
		}),
	)

	switch tp {
	case node.Light, node.Full, node.Bridge:
		return fx.Module(
			"state",
			baseComponents,
		)
	default:
		panic("invalid node type")
	}
}

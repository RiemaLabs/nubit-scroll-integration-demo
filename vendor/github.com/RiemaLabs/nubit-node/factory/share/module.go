package share

import (
	"context"

	"github.com/ipfs/go-datastore"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/p2p/net/conngater"
	"go.uber.org/fx"

	libhead "github.com/RiemaLabs/go-libp2p-header"
	"github.com/RiemaLabs/go-libp2p-header/sync"

	share "github.com/RiemaLabs/nubit-node/da"
	"github.com/RiemaLabs/nubit-node/da/full"
	"github.com/RiemaLabs/nubit-node/da/light"
	"github.com/RiemaLabs/nubit-node/factory/node"
	modp2p "github.com/RiemaLabs/nubit-node/factory/p2p"
	disc "github.com/RiemaLabs/nubit-node/p2p/discovery"
	"github.com/RiemaLabs/nubit-node/p2p/p2peds"
	"github.com/RiemaLabs/nubit-node/p2p/p2pnd"
	"github.com/RiemaLabs/nubit-node/p2p/p2psub"
	"github.com/RiemaLabs/nubit-node/p2p/peers"
	"github.com/RiemaLabs/nubit-node/rtrv/eds"
	getters "github.com/RiemaLabs/nubit-node/rtrv/rtrv"
	header "github.com/RiemaLabs/nubit-node/strucs/eh"
)

func ConstructModule(tp node.Type, cfg *Config, options ...fx.Option) fx.Option {
	// sanitize config values before constructing module
	cfgErr := cfg.Validate(tp)

	baseComponents := fx.Options(
		fx.Supply(*cfg),
		fx.Error(cfgErr),
		fx.Options(options...),
		fx.Provide(newShareModule),
		peerManagerComponents(tp, cfg),
		discoveryComponents(cfg),
		p2pSubComponents(),
	)

	bridgeAndFullComponents := fx.Options(
		fx.Provide(getters.NewStoreGetter),
		p2pServerComponents(cfg),
		//edsStoreComponents(cfg),
		fullAvailabilityComponents(),
		p2pGetterComponents(cfg),
		fx.Provide(func(p2pSub *p2psub.PubSub) p2psub.BroadcastFn {
			return p2pSub.Broadcast
		}),
	)

	switch tp {
	case node.Bridge:
		return fx.Module(
			"share",
			baseComponents,
			bridgeAndFullComponents,
			fx.Provide(func() peers.Parameters {
				return cfg.PeerManagerParams
			}),
			fx.Provide(bridgeGetter),
			fx.Invoke(func(lc fx.Lifecycle, sub *p2psub.PubSub) error {
				lc.Append(fx.Hook{
					OnStart: sub.Start,
					OnStop:  sub.Stop,
				})
				return nil
			}),
		)
	case node.Full:
		return fx.Module(
			"share",
			baseComponents,
			bridgeAndFullComponents,
			fx.Provide(getters.NewIPLDGetter),
			fx.Provide(fullGetter),
		)
	case node.Light:
		return fx.Module(
			"share",
			baseComponents,
			p2pGetterComponents(cfg),
			lightAvailabilityComponents(cfg),
			fx.Invoke(ensureEmptyEDSInBS),
			fx.Provide(getters.NewIPLDGetter),
			fx.Provide(lightGetter),
			// p2psub broadcaster stub for daser
			fx.Provide(func() p2psub.BroadcastFn {
				return func(context.Context, p2psub.Notification) error {
					return nil
				}
			}),
		)
	default:
		panic("invalid node type")
	}
}

func discoveryComponents(cfg *Config) fx.Option {
	return fx.Options(
		fx.Invoke(func(_ *disc.Discovery) {}),
		fx.Provide(fx.Annotate(
			newDiscovery(cfg.Discovery),
			fx.OnStart(func(ctx context.Context, d *disc.Discovery) error {
				return d.Start(ctx)
			}),
			fx.OnStop(func(ctx context.Context, d *disc.Discovery) error {
				return d.Stop(ctx)
			}),
		)),
	)
}

func peerManagerComponents(tp node.Type, cfg *Config) fx.Option {
	switch tp {
	case node.Full, node.Light:
		return fx.Options(
			fx.Provide(func() peers.Parameters {
				return cfg.PeerManagerParams
			}),
			fx.Provide(
				func(
					params peers.Parameters,
					host host.Host,
					connGater *conngater.BasicConnectionGater,
					p2pSub *p2psub.PubSub,
					headerSub libhead.Subscriber[*header.ExtendedHeader],
					// we must ensure Syncer is started before PeerManager
					// so that Syncer registers header validator before PeerManager subscribes to headers
					_ *sync.Syncer[*header.ExtendedHeader],
				) (*peers.Manager, error) {
					return peers.NewManager(
						params,
						host,
						connGater,
						peers.WithP2pSubPools(p2pSub, headerSub),
					)
				},
			),
		)
	case node.Bridge:
		return fx.Provide(peers.NewManager)
	default:
		panic("invalid node type")
	}
}

func p2pSubComponents() fx.Option {
	return fx.Provide(
		func(ctx context.Context, h host.Host, network modp2p.Network) (*p2psub.PubSub, error) {
			return p2psub.NewPubSub(ctx, h, network.String())
		},
	)
}

// p2pGetterComponents provides components for a p2p getter that
// is capable of requesting
func p2pGetterComponents(cfg *Config) fx.Option {
	return fx.Options(
		// p2p-nd client
		fx.Provide(
			func(host host.Host, network modp2p.Network) (*p2pnd.Client, error) {
				cfg.P2PNDParams.WithNetworkID(network.String())
				return p2pnd.NewClient(cfg.P2PNDParams, host)
			},
		),

		// p2p-eds client
		fx.Provide(
			func(host host.Host, network modp2p.Network) (*p2peds.Client, error) {
				cfg.P2PEDSParams.WithNetworkID(network.String())
				return p2peds.NewClient(cfg.P2PEDSParams, host)
			},
		),

		fx.Provide(fx.Annotate(
			getters.NewP2pGetter,
			fx.OnStart(func(ctx context.Context, getter *getters.P2pGetter) error {
				return getter.Start(ctx)
			}),
			fx.OnStop(func(ctx context.Context, getter *getters.P2pGetter) error {
				return getter.Stop(ctx)
			}),
		)),
	)
}

func p2pServerComponents(cfg *Config) fx.Option {
	return fx.Options(
		fx.Invoke(func(_ *p2peds.Server, _ *p2pnd.Server) {}),
		fx.Provide(fx.Annotate(
			func(host host.Host, store *eds.Store, network modp2p.Network) (*p2peds.Server, error) {
				cfg.P2PEDSParams.WithNetworkID(network.String())
				return p2peds.NewServer(cfg.P2PEDSParams, host, store)
			},
			fx.OnStart(func(ctx context.Context, server *p2peds.Server) error {
				return server.Start(ctx)
			}),
			fx.OnStop(func(ctx context.Context, server *p2peds.Server) error {
				return server.Stop(ctx)
			}),
		)),
		fx.Provide(fx.Annotate(
			func(
				host host.Host,
				store *eds.Store,
				network modp2p.Network,
			) (*p2pnd.Server, error) {
				cfg.P2PNDParams.WithNetworkID(network.String())
				return p2pnd.NewServer(cfg.P2PNDParams, host, store)
			},
			fx.OnStart(func(ctx context.Context, server *p2pnd.Server) error {
				return server.Start(ctx)
			}),
			fx.OnStop(func(ctx context.Context, server *p2pnd.Server) error {
				return server.Stop(ctx)
			})),
		),
	)
}

func edsStoreComponents(cfg *Config) fx.Option {
	return fx.Options(
		fx.Provide(fx.Annotate(
			func(path node.StorePath, ds datastore.Batching) (*eds.Store, error) {
				return eds.NewStore(cfg.EDSStoreParams, string(path), ds)
			},
			fx.OnStart(func(ctx context.Context, store *eds.Store) error {
				err := store.Start(ctx)
				if err != nil {
					return err
				}
				return ensureEmptyCARExists(ctx, store)
			}),
			fx.OnStop(func(ctx context.Context, store *eds.Store) error {
				return store.Stop(ctx)
			}),
		)),
	)
}

func fullAvailabilityComponents() fx.Option {
	return fx.Options(
		fx.Provide(fx.Annotate(
			full.NewShareAvailability,
			fx.OnStart(func(ctx context.Context, avail *full.ShareAvailability) error {
				return avail.Start(ctx)
			}),
			fx.OnStop(func(ctx context.Context, avail *full.ShareAvailability) error {
				return avail.Stop(ctx)
			}),
		)),
		fx.Provide(func(avail *full.ShareAvailability) share.Availability {
			return avail
		}),
	)
}

func lightAvailabilityComponents(cfg *Config) fx.Option {
	return fx.Options(
		fx.Provide(fx.Annotate(
			light.NewShareAvailability,
			fx.OnStop(func(ctx context.Context, la *light.ShareAvailability) error {
				return la.Close(ctx)
			}),
		)),
		fx.Provide(func() []light.Option {
			return []light.Option{
				light.WithSampleAmount(cfg.LightAvailability.SampleAmount),
			}
		}),
		fx.Provide(func(avail *light.ShareAvailability) share.Availability {
			return avail
		}),
	)
}

package share

import (
	"context"
	"errors"

	"github.com/filecoin-project/dagstore"
	"github.com/ipfs/boxo/blockservice"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/routing"
	routingdisc "github.com/libp2p/go-libp2p/p2p/discovery/routing"

	"github.com/RiemaLabs/nubit-validator/da/da"

	"github.com/RiemaLabs/nubit-node/da"
	"github.com/RiemaLabs/nubit-node/rtrv/eds"
	"github.com/RiemaLabs/nubit-node/rtrv/rtrv"
	"github.com/RiemaLabs/nubit-node/rtrv/lp2p"
	disc "github.com/RiemaLabs/nubit-node/p2p/discovery"
	"github.com/RiemaLabs/nubit-node/p2p/peers"
)

const (
	// fullNodesTag is the tag used to identify full nodes in the discovery service.
	fullNodesTag = "full"
)

func newDiscovery(cfg *disc.Parameters,
) func(routing.ContentRouting, host.Host, *peers.Manager) (*disc.Discovery, error) {
	return func(
		r routing.ContentRouting,
		h host.Host,
		manager *peers.Manager,
	) (*disc.Discovery, error) {
		return disc.NewDiscovery(
			cfg,
			h,
			routingdisc.NewRoutingDiscovery(r),
			fullNodesTag,
			disc.WithOnPeersUpdate(manager.UpdateNodePool),
		)
	}
}

func newShareModule(getter share.Getter, avail share.Availability) Module {
	return &module{getter, avail}
}

// ensureEmptyCARExists adds an empty EDS to the provided EDS store.
func ensureEmptyCARExists(ctx context.Context, store *eds.Store) error {
	emptyEDS := share.EmptyExtendedDataSquare()
	emptyDAH, err := da.NewDataAvailabilityHeader(emptyEDS)
	if err != nil {
		return err
	}

	err = store.Put(ctx, emptyDAH.Hash(), emptyEDS)
	if errors.Is(err, dagstore.ErrShardExists) {
		return nil
	}
	return err
}

// ensureEmptyEDSInBS checks if the given DAG contains an empty block data square.
// If it does not, it stores an empty block. This optimization exists to prevent
// redundant storing of empty block data so that it is only stored once and returned
// upon request for a block with an empty data square.
func ensureEmptyEDSInBS(ctx context.Context, bServ blockservice.BlockService) error {
	_, err := ipld.AddShares(ctx, share.EmptyBlockShares(), bServ)
	return err
}

func lightGetter(
	p2pGetter *getters.P2pGetter,
	ipldGetter *getters.IPLDGetter,
	cfg Config,
) share.Getter {
	var cascade []share.Getter
	if cfg.UseShareExchange {
		cascade = append(cascade, p2pGetter)
	}
	cascade = append(cascade, ipldGetter)
	return getters.NewCascadeGetter(cascade)
}

// P2pGetter is added to bridge nodes for the case that a shard is removed
// after detected shard corruption. This ensures the block is fetched and stored
// by p2p the next time the data is retrieved (meaning shard recovery is
// manual after corruption is detected).
func bridgeGetter(
	storeGetter *getters.StoreGetter,
	p2pGetter *getters.P2pGetter,
	cfg Config,
) share.Getter {
	var cascade []share.Getter
	cascade = append(cascade, storeGetter)
	if cfg.UseShareExchange {
		cascade = append(cascade, p2pGetter)
	}
	return getters.NewCascadeGetter(cascade)
}

func fullGetter(
	storeGetter *getters.StoreGetter,
	p2pGetter *getters.P2pGetter,
	ipldGetter *getters.IPLDGetter,
	cfg Config,
) share.Getter {
	var cascade []share.Getter
	cascade = append(cascade, storeGetter)
	if cfg.UseShareExchange {
		cascade = append(cascade, p2pGetter)
	}
	cascade = append(cascade, ipldGetter)
	return getters.NewCascadeGetter(cascade)
}

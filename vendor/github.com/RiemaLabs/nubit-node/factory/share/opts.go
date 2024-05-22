package share

import (
	"github.com/RiemaLabs/nubit-node/rtrv/eds"
	"github.com/RiemaLabs/nubit-node/rtrv/rtrv"
	disc "github.com/RiemaLabs/nubit-node/p2p/discovery"
	"github.com/RiemaLabs/nubit-node/p2p/peers"
	"github.com/RiemaLabs/nubit-node/p2p/p2peds"
	"github.com/RiemaLabs/nubit-node/p2p/p2pnd"
)

// WithPeerManagerMetrics is a utility function to turn on peer manager metrics and that is
// expected to be "invoked" by the fx lifecycle.
func WithPeerManagerMetrics(m *peers.Manager) error {
	return m.WithMetrics()
}

// WithDiscoveryMetrics is a utility function to turn on discovery metrics and that is expected to
// be "invoked" by the fx lifecycle.
func WithDiscoveryMetrics(d *disc.Discovery) error {
	return d.WithMetrics()
}

func WithP2pClientMetrics(edsClient *p2peds.Client, ndClient *p2pnd.Client) error {
	err := edsClient.WithMetrics()
	if err != nil {
		return err
	}

	return ndClient.WithMetrics()
}

func WithP2pServerMetrics(edsServer *p2peds.Server, ndServer *p2pnd.Server) error {
	err := edsServer.WithMetrics()
	if err != nil {
		return err
	}

	return ndServer.WithMetrics()
}

func WithP2pGetterMetrics(sg *getters.P2pGetter) error {
	return sg.WithMetrics()
}

func WithStoreMetrics(s *eds.Store) error {
	return s.WithMetrics()
}

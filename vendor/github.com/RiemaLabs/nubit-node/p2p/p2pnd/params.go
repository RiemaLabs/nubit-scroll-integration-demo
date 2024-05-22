package p2pnd

import (
	"fmt"

	logging "github.com/ipfs/go-log/v2"

	"github.com/RiemaLabs/nubit-node/p2p"
)

const protocolString = "/p2p/nd/v0.0.3"

var log = logging.Logger("p2p/nd")

// Parameters is the set of parameters that must be configured for the p2p/eds protocol.
type Parameters = p2p.Parameters

func DefaultParameters() *Parameters {
	return p2p.DefaultParameters()
}

func (c *Client) WithMetrics() error {
	metrics, err := p2p.InitClientMetrics("nd")
	if err != nil {
		return fmt.Errorf("p2p/nd: init Metrics: %w", err)
	}
	c.metrics = metrics
	return nil
}

func (srv *Server) WithMetrics() error {
	metrics, err := p2p.InitServerMetrics("nd")
	if err != nil {
		return fmt.Errorf("p2p/nd: init Metrics: %w", err)
	}
	srv.metrics = metrics
	return nil
}

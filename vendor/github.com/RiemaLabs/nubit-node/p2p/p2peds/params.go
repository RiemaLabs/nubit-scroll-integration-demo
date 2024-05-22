package p2peds

import (
	"fmt"

	logging "github.com/ipfs/go-log/v2"

	"github.com/RiemaLabs/nubit-node/p2p"
)

const protocolString = "/p2p/eds/v0.0.1"

var log = logging.Logger("p2p/eds")

// Parameters is the set of parameters that must be configured for the p2p/eds protocol.
type Parameters struct {
	*p2p.Parameters

	// BufferSize defines the size of the buffer used for writing an ODS over the stream.
	BufferSize uint64
}

func DefaultParameters() *Parameters {
	return &Parameters{
		Parameters: p2p.DefaultParameters(),
		BufferSize: 32 * 1024,
	}
}

func (p *Parameters) Validate() error {
	if p.BufferSize <= 0 {
		return fmt.Errorf("invalid buffer size: %v, value should be positive and non-zero", p.BufferSize)
	}

	return p.Parameters.Validate()
}

func (c *Client) WithMetrics() error {
	metrics, err := p2p.InitClientMetrics("eds")
	if err != nil {
		return fmt.Errorf("p2p/eds: init Metrics: %w", err)
	}
	c.metrics = metrics
	return nil
}

func (s *Server) WithMetrics() error {
	metrics, err := p2p.InitServerMetrics("eds")
	if err != nil {
		return fmt.Errorf("p2p/eds: init Metrics: %w", err)
	}
	s.metrics = metrics
	return nil
}

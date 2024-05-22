package share

import (
	"fmt"

	"github.com/RiemaLabs/nubit-node/da/light"
	"github.com/RiemaLabs/nubit-node/factory/node"
	"github.com/RiemaLabs/nubit-node/p2p/discovery"
	"github.com/RiemaLabs/nubit-node/p2p/p2peds"
	"github.com/RiemaLabs/nubit-node/p2p/p2pnd"
	"github.com/RiemaLabs/nubit-node/p2p/peers"
	"github.com/RiemaLabs/nubit-node/rtrv/eds"
)

// TODO: some params are pointers and other are not, Let's fix this.
type Config struct {
	// EDSStoreParams sets eds store configuration parameters
	EDSStoreParams *eds.Parameters

	UseShareExchange bool
	// P2PEDSParams sets p2peds client and server configuration parameters
	P2PEDSParams *p2peds.Parameters

	// P2PNDParams sets p2pnd client and server configuration parameters
	P2PNDParams *p2pnd.Parameters
	// PeerManagerParams sets peer-manager configuration parameters
	PeerManagerParams peers.Parameters

	LightAvailability light.Parameters `toml:",omitempty"`
	Discovery         *discovery.Parameters
}

func DefaultConfig(tp node.Type) Config {
	cfg := Config{
		EDSStoreParams:    eds.DefaultParameters(),
		Discovery:         discovery.DefaultParameters(),
		P2PEDSParams:      p2peds.DefaultParameters(),
		P2PNDParams:       p2pnd.DefaultParameters(),
		UseShareExchange:  true,
		PeerManagerParams: peers.DefaultParameters(),
	}

	if tp == node.Light {
		cfg.LightAvailability = light.DefaultParameters()
	}

	return cfg
}

// Validate performs basic validation of the config.
func (cfg *Config) Validate(tp node.Type) error {
	if tp == node.Light {
		if err := cfg.LightAvailability.Validate(); err != nil {
			return fmt.Errorf("nodebuilder/share: %w", err)
		}
	}

	if err := cfg.Discovery.Validate(); err != nil {
		return fmt.Errorf("nodebuilder/share: %w", err)
	}

	if err := cfg.P2PNDParams.Validate(); err != nil {
		return fmt.Errorf("nodebuilder/share: %w", err)
	}

	if err := cfg.P2PEDSParams.Validate(); err != nil {
		return fmt.Errorf("nodebuilder/share: %w", err)
	}

	if err := cfg.PeerManagerParams.Validate(); err != nil {
		return fmt.Errorf("nodebuilder/share: %w", err)
	}

	return nil
}

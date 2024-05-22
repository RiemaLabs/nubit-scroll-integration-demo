package state

import (
	"github.com/RiemaLabs/go-libp2p-header/sync"
	apptypes "github.com/RiemaLabs/nubit-validator/utils/tk/sh/blob/types"

	"github.com/RiemaLabs/nubit-node/factory/core"
	header "github.com/RiemaLabs/nubit-node/strucs/eh"
	"github.com/RiemaLabs/nubit-node/strucs/state"
)

// coreAccessor constructs a new instance of state.Module over
// a nubit-core connection.
func coreAccessor(
	corecfg core.Config,
	signer *apptypes.KeyringSigner,
	sync *sync.Syncer[*header.ExtendedHeader],
) (*state.CoreAccessor, Module) {
	ca := state.NewCoreAccessor(signer, sync, corecfg.IP, corecfg.RPCPort, corecfg.GRPCPort)

	return ca, ca
}

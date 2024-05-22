package state

import (
	"go.uber.org/fx"

	"github.com/RiemaLabs/nubit-validator/utils/tk/sh/blob/types"
)

// WithKeyringSigner overrides the default keyring signer constructed
// by the node.
func WithKeyringSigner(signer *types.KeyringSigner) fx.Option {
	return fx.Replace(signer)
}

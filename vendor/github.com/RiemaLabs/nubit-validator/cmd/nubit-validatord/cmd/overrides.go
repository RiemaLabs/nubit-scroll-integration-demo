package cmd

import (
	app "github.com/RiemaLabs/nubit-validator/tx"
	"github.com/cosmos/cosmos-sdk/server"
	"github.com/spf13/cobra"
)

// setDefaultConsensusParams sets the default consensus parameters for the
// embedded server context.
func setDefaultConsensusParams(command *cobra.Command) error {
	ctx := server.GetServerContextFromCmd(command)
	ctx.DefaultConsensusParams = app.DefaultConsensusParams()
	return server.SetCmdServerContext(command, ctx)
}

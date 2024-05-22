package client

import (
	"context"
	"fmt"
	"net/http"

	"github.com/filecoin-project/go-jsonrpc"

	"github.com/RiemaLabs/nubit-node/factory/blob"
	"github.com/RiemaLabs/nubit-node/factory/da"
	"github.com/RiemaLabs/nubit-node/factory/das"
	"github.com/RiemaLabs/nubit-node/factory/header"
	"github.com/RiemaLabs/nubit-node/factory/node"
	"github.com/RiemaLabs/nubit-node/factory/p2p"
	"github.com/RiemaLabs/nubit-node/factory/share"
	"github.com/RiemaLabs/nubit-node/factory/state"
	"github.com/RiemaLabs/nubit-node/rpc/rpc/perms"
)

var (
	// staticClient is used for generating the OpenRPC spec.
	staticClient Client
	Modules      = moduleMap(&staticClient)
)

type Client struct {
	Header header.API
	State  state.API
	Share  share.API
	DAS    das.API
	P2P    p2p.API
	Node   node.API
	Blob   blob.API
	DA     da.API

	closer multiClientCloser
}

// multiClientCloser is a wrapper struct to close clients across multiple namespaces.
type multiClientCloser struct {
	closers []jsonrpc.ClientCloser
}

// register adds a new closer to the multiClientCloser
func (m *multiClientCloser) register(closer jsonrpc.ClientCloser) {
	m.closers = append(m.closers, closer)
}

// closeAll closes all saved clients.
func (m *multiClientCloser) closeAll() {
	for _, closer := range m.closers {
		closer()
	}
}

// Close closes the connections to all namespaces registered on the staticClient.
func (c *Client) Close() {
	c.closer.closeAll()
}

// NewClient creates a new Client with one connection per namespace with the
// given token as the authorization token.
func NewClient(ctx context.Context, addr string, token string) (*Client, error) {
	authHeader := http.Header{perms.AuthKey: []string{fmt.Sprintf("Bearer %s", token)}}
	return newClient(ctx, addr, authHeader)
}

func newClient(ctx context.Context, addr string, authHeader http.Header) (*Client, error) {
	var multiCloser multiClientCloser
	var client Client
	for name, module := range moduleMap(&client) {
		closer, err := jsonrpc.NewClient(ctx, addr, name, module, authHeader)
		if err != nil {
			return nil, err
		}
		multiCloser.register(closer)
	}

	return &client, nil
}

func moduleMap(client *Client) map[string]interface{} {
	// TODO: this duplication of strings many times across the codebase can be avoided with issue #1176
	return map[string]interface{}{
		"share":  &client.Share.Internal,
		"state":  &client.State.Internal,
		"header": &client.Header.Internal,
		"das":    &client.DAS.Internal,
		"p2p":    &client.P2P.Internal,
		"node":   &client.Node.Internal,
		"blob":   &client.Blob.Internal,
		"da":     &client.DA.Internal,
	}
}

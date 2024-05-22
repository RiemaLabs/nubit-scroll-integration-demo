package p2pnd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"

	"github.com/RiemaLabs/go-libp2p-messenger/serde"
	kzg "github.com/RiemaLabs/nubit-kzg"

	"github.com/RiemaLabs/nubit-node/da"
	"github.com/RiemaLabs/nubit-node/p2p"
	pb "github.com/RiemaLabs/nubit-node/p2p/p2pnd/pb"
)

// Client implements client side of p2p/nd protocol to obtain namespaced shares data from remote
// peers.
type Client struct {
	params     *Parameters
	protocolID protocol.ID

	host    host.Host
	metrics *p2p.Metrics
}

// NewClient creates a new shrEx/nd client
func NewClient(params *Parameters, host host.Host) (*Client, error) {
	if err := params.Validate(); err != nil {
		return nil, fmt.Errorf("p2p-nd: client creation failed: %w", err)
	}

	return &Client{
		host:       host,
		protocolID: p2p.ProtocolID(params.NetworkID(), protocolString),
		params:     params,
	}, nil
}

// RequestND requests namespaced data from the given peer.
// Returns NamespacedShares with unverified inclusion proofs against the share.Root.
func (c *Client) RequestND(
	ctx context.Context,
	root *share.Root,
	namespace share.Namespace,
	peer peer.ID,
) (share.NamespacedShares, error) {
	if err := namespace.ValidateForData(); err != nil {
		return nil, err
	}

	shares, err := c.doRequest(ctx, root, namespace, peer)
	if err == nil {
		return shares, nil
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		c.metrics.ObserveRequests(ctx, 1, p2p.StatusTimeout)
		return nil, err
	}
	// some net.Errors also mean the context deadline was exceeded, but yamux/mocknet do not
	// unwrap to a ctx err
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		if deadline, _ := ctx.Deadline(); deadline.Before(time.Now()) {
			c.metrics.ObserveRequests(ctx, 1, p2p.StatusTimeout)
			return nil, context.DeadlineExceeded
		}
	}
	if err != p2p.ErrNotFound && err != p2p.ErrRateLimited {
		log.Warnw("client-nd: peer returned err", "err", err)
	}
	return nil, err
}

func (c *Client) doRequest(
	ctx context.Context,
	root *share.Root,
	namespace share.Namespace,
	peerID peer.ID,
) (share.NamespacedShares, error) {
	stream, err := c.host.NewStream(ctx, peerID, c.protocolID)
	if err != nil {
		return nil, err
	}
	defer stream.Close()

	c.setStreamDeadlines(ctx, stream)

	req := &pb.GetSharesByNamespaceRequest{
		RootHash:  root.Hash(),
		Namespace: namespace,
	}

	_, err = serde.Write(stream, req)
	if err != nil {
		c.metrics.ObserveRequests(ctx, 1, p2p.StatusSendReqErr)
		stream.Reset() //nolint:errcheck
		return nil, fmt.Errorf("client-nd: writing request: %w", err)
	}

	err = stream.CloseWrite()
	if err != nil {
		log.Debugw("client-nd: closing write side of the stream", "err", err)
	}

	if err := c.readStatus(ctx, stream); err != nil {
		return nil, err
	}
	return c.readNamespacedShares(ctx, stream)
}

func (c *Client) readStatus(ctx context.Context, stream network.Stream) error {
	var resp pb.GetSharesByNamespaceStatusResponse
	_, err := serde.Read(stream, &resp)
	if err != nil {
		// server is overloaded and closed the stream
		if errors.Is(err, io.EOF) {
			c.metrics.ObserveRequests(ctx, 1, p2p.StatusRateLimited)
			return p2p.ErrRateLimited
		}
		c.metrics.ObserveRequests(ctx, 1, p2p.StatusReadRespErr)
		stream.Reset() //nolint:errcheck
		return fmt.Errorf("client-nd: reading status response: %w", err)
	}

	return c.convertStatusToErr(ctx, resp.Status)
}

// readNamespacedShares converts proto Rows to share.NamespacedShares
func (c *Client) readNamespacedShares(
	ctx context.Context,
	stream network.Stream,
) (share.NamespacedShares, error) {
	var shares share.NamespacedShares
	for {
		var row pb.NamespaceRowResponse
		_, err := serde.Read(stream, &row)
		if err != nil {
			if errors.Is(err, io.EOF) {
				// all data is received and steam is closed by server
				return shares, nil
			}
			c.metrics.ObserveRequests(ctx, 1, p2p.StatusReadRespErr)
			return nil, err
		}
		var proof kzg.NamespaceRangeProof
		if row.Proof != nil {
			if len(row.Shares) != 0 {
				proof = kzg.NewInclusionNamespaceRangeProof(
					int(row.Proof.Start),
					int(row.Proof.End),
					nil,
					false,
				)
			} else {
				proof = kzg.NewAbsenceNamespaceRangeProof(
					int(row.Proof.Start),
					int(row.Proof.End),
					nil,
					nil,
					false,
				)
			}
		}
		shares = append(shares, share.NamespacedRow{
			Shares: row.Shares,
			Proof:  &proof,
		})
	}
}

func (c *Client) setStreamDeadlines(ctx context.Context, stream network.Stream) {
	// set read/write deadline to use context deadline if it exists
	deadline, ok := ctx.Deadline()
	if ok {
		err := stream.SetDeadline(deadline)
		if err == nil {
			return
		}
		log.Debugw("client-nd: set stream deadline", "err", err)
	}

	// if deadline not set, client read deadline defaults to server write deadline
	if c.params.ServerWriteTimeout != 0 {
		err := stream.SetReadDeadline(time.Now().Add(c.params.ServerWriteTimeout))
		if err != nil {
			log.Debugw("client-nd: set read deadline", "err", err)
		}
	}

	// if deadline not set, client write deadline defaults to server read deadline
	if c.params.ServerReadTimeout != 0 {
		err := stream.SetWriteDeadline(time.Now().Add(c.params.ServerReadTimeout))
		if err != nil {
			log.Debugw("client-nd: set write deadline", "err", err)
		}
	}
}

func (c *Client) convertStatusToErr(ctx context.Context, status pb.StatusCode) error {
	switch status {
	case pb.StatusCode_OK:
		c.metrics.ObserveRequests(ctx, 1, p2p.StatusSuccess)
		return nil
	case pb.StatusCode_NOT_FOUND:
		c.metrics.ObserveRequests(ctx, 1, p2p.StatusNotFound)
		return p2p.ErrNotFound
	case pb.StatusCode_INVALID:
		log.Warn("client-nd: invalid request")
		fallthrough
	case pb.StatusCode_INTERNAL:
		fallthrough
	default:
		return p2p.ErrInvalidResponse
	}
}

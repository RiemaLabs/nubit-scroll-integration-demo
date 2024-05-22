package blob

import (
	"context"

	"github.com/RiemaLabs/nubit-node/da"
	"github.com/RiemaLabs/nubit-node/strucs/btx"
)

var _ Module = (*API)(nil)

// Module defines the API related to interacting with the blobs
//
//go:generate mockgen -destination=mocks/api.go -package=mocks . Module
type Module interface {
	// Submit sends Blobs and reports the height in which they were included.
	// Allows sending multiple Blobs atomically synchronously.
	// Uses default wallet registered on the Node.
	Submit(_ context.Context, _ []*blob.Blob, _ blob.GasPrice) (height uint64, _ error)
	// Get retrieves the blob by KZG commitment under the given namespace and height.
	Get(_ context.Context, height uint64, _ share.Namespace, _ blob.KzgCommitment) (*blob.Blob, error)
	// GetAll returns all blobs at the given height under the given namespaces.
	GetAll(_ context.Context, height uint64, _ []share.Namespace) ([]*blob.Blob, error)
	// GetProof retrieves proofs in the given namespaces at the given height by commitment.
	GetProof(_ context.Context, height uint64, _ share.Namespace, _ blob.KzgCommitment) (*blob.Proof, error)
	// Included checks whether a blob's given commitment(Merkle subtree root) is included at
	// given height and under the namespace.
	Included(_ context.Context, height uint64, _ share.Namespace, _ *blob.Proof, _ blob.KzgCommitment) (bool, error)
}

type API struct {
	Internal struct {
		Submit   func(context.Context, []*blob.Blob, blob.GasPrice) (uint64, error)                         `perm:"write"`
		Get      func(context.Context, uint64, share.Namespace, blob.KzgCommitment) (*blob.Blob, error)        `perm:"read"`
		GetAll   func(context.Context, uint64, []share.Namespace) ([]*blob.Blob, error)                     `perm:"read"`
		GetProof func(context.Context, uint64, share.Namespace, blob.KzgCommitment) (*blob.Proof, error)       `perm:"read"`
		Included func(context.Context, uint64, share.Namespace, *blob.Proof, blob.KzgCommitment) (bool, error) `perm:"read"`
	}
}

func (api *API) Submit(ctx context.Context, blobs []*blob.Blob, gasPrice blob.GasPrice) (uint64, error) {
	return api.Internal.Submit(ctx, blobs, gasPrice)
}

func (api *API) Get(
	ctx context.Context,
	height uint64,
	namespace share.Namespace,
	commitment blob.KzgCommitment,
) (*blob.Blob, error) {
	return api.Internal.Get(ctx, height, namespace, commitment)
}

func (api *API) GetAll(ctx context.Context, height uint64, namespaces []share.Namespace) ([]*blob.Blob, error) {
	return api.Internal.GetAll(ctx, height, namespaces)
}

func (api *API) GetProof(
	ctx context.Context,
	height uint64,
	namespace share.Namespace,
	commitment blob.KzgCommitment,
) (*blob.Proof, error) {
	return api.Internal.GetProof(ctx, height, namespace, commitment)
}

func (api *API) Included(
	ctx context.Context,
	height uint64,
	namespace share.Namespace,
	proof *blob.Proof,
	commitment blob.KzgCommitment,
) (bool, error) {
	return api.Internal.Included(ctx, height, namespace, proof, commitment)
}

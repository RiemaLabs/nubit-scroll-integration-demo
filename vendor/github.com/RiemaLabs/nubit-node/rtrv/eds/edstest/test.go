package edstest

import (
	"github.com/stretchr/testify/require"

	"github.com/RiemaLabs/nubit-validator/utils/wrapper"
	"github.com/RiemaLabs/rsmt2d"

	share "github.com/RiemaLabs/nubit-node/da"
	"github.com/RiemaLabs/nubit-node/da/sharetest"
)

// RandEDS generates EDS filled with the random data with the given size for original square. It
// uses require.TestingT to be able to take both a *testing.T and a *testing.B.
func RandEDS(t require.TestingT, size int) *rsmt2d.ExtendedDataSquare {
	shares := sharetest.RandShares(t, size*size)
	eds, err := rsmt2d.ComputeExtendedDataSquare(shares, share.DefaultRSMT2DCodec(), wrapper.NewConstructor(uint64(size)))
	require.NoError(t, err, "failure to recompute the extended data square")
	return eds
}

func RandEDSWithNamespace(
	t require.TestingT,
	namespace share.Namespace,
	size int,
) (*rsmt2d.ExtendedDataSquare, *share.Root) {
	shares := sharetest.RandSharesWithNamespace(t, namespace, size*size)
	eds, err := rsmt2d.ComputeExtendedDataSquare(shares, share.DefaultRSMT2DCodec(), wrapper.NewConstructor(uint64(size)))
	require.NoError(t, err, "failure to recompute the extended data square")
	dah, err := share.NewRoot(eds)
	require.NoError(t, err)
	return eds, dah
}

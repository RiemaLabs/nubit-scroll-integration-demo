package das

import (
	"context"
	"fmt"
	"time"

	"github.com/ipfs/go-datastore"

	libhead "github.com/RiemaLabs/go-libp2p-header"

	share "github.com/RiemaLabs/nubit-node/da"
	"github.com/RiemaLabs/nubit-node/da/das"
	pruner "github.com/RiemaLabs/nubit-node/man"
	p2psub "github.com/RiemaLabs/nubit-node/p2p/p2psub"
	header "github.com/RiemaLabs/nubit-node/strucs/eh"
)

var _ Module = (*daserStub)(nil)

var errStub = fmt.Errorf("module/das: stubbed: dasing is not available on bridge nodes")

// daserStub is a stub implementation of the DASer that is used on bridge nodes, so that we can
// provide a friendlier error when users try to access the daser over the API.
type daserStub struct{}

func (d daserStub) SamplingStats(context.Context) (das.SamplingStats, error) {
	return das.SamplingStats{}, errStub
}

func (d daserStub) WaitCatchUp(context.Context) error {
	return errStub
}

func newDaserStub() Module {
	return &daserStub{}
}

func newDASer(
	da share.Availability,
	hsub libhead.Subscriber[*header.ExtendedHeader],
	store libhead.Store[*header.ExtendedHeader],
	batching datastore.Batching,
	bFn p2psub.BroadcastFn,
	availWindow pruner.AvailabilityWindow,
	options ...das.Option,
) (*das.DASer, error) {
	options = append(options, das.WithSamplingWindow(time.Duration(availWindow)))

	ds, err := das.NewDASer(da, hsub, store, batching, bFn, options...)
	if err != nil {
		return nil, err
	}

	return ds, nil
}

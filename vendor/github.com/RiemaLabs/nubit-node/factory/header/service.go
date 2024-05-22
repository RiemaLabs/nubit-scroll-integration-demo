package header

import (
	"context"
	"fmt"

	libhead "github.com/RiemaLabs/go-libp2p-header"
	"github.com/RiemaLabs/go-libp2p-header/p2p"
	"github.com/RiemaLabs/go-libp2p-header/sync"
	app "github.com/RiemaLabs/nubit-validator/tx"

	"github.com/RiemaLabs/nubit-node/rtrv/eds"
	header "github.com/RiemaLabs/nubit-node/strucs/eh"
)

// Service represents the header Service that can be started / stopped on a node.
// Service's main function is to manage its sub-services. Service can contain several
// sub-services, such as Exchange, ExchangeServer, Syncer, and so forth.
type Service struct {
	ex libhead.Exchange[*header.ExtendedHeader]

	syncer    syncer
	sub       libhead.Subscriber[*header.ExtendedHeader]
	p2pServer *p2p.ExchangeServer[*header.ExtendedHeader]
	store     libhead.Store[*header.ExtendedHeader]
	edsStore  *eds.Store
}

// syncer bare minimum Syncer interface for testing
type syncer interface {
	libhead.Head[*header.ExtendedHeader]

	State() sync.State
	SyncWait(ctx context.Context) error
}

// newHeaderService creates a new instance of header Service.
func newHeaderService(
	syncer *sync.Syncer[*header.ExtendedHeader],
	sub libhead.Subscriber[*header.ExtendedHeader],
	p2pServer *p2p.ExchangeServer[*header.ExtendedHeader],
	ex libhead.Exchange[*header.ExtendedHeader],
	store libhead.Store[*header.ExtendedHeader],
	edsStore *eds.Store,
) Module {
	return &Service{
		syncer:    syncer,
		sub:       sub,
		p2pServer: p2pServer,
		ex:        ex,
		store:     store,
		edsStore:  edsStore,
	}
}

func (s *Service) GetByHash(ctx context.Context, hash libhead.Hash) (*header.ExtendedHeader, error) {
	return s.store.Get(ctx, hash)
}

func (s *Service) GetRangeByHeight(
	ctx context.Context,
	from *header.ExtendedHeader,
	to uint64,
) ([]*header.ExtendedHeader, error) {
	return s.store.GetRangeByHeight(ctx, from, to)
}

func (s *Service) GetByHeight(ctx context.Context, height uint64) (*header.ExtendedHeader, error) {
	head, err := s.syncer.Head(ctx)
	switch {
	case err != nil:
		return nil, err
	case head.Height() == height:
		return head, nil
	case head.Height()+1 < height:
		return nil, fmt.Errorf("header: given height is from the future: "+
			"networkHeight: %d, requestedHeight: %d", head.Height(), height)
	}

	// TODO(vgonkivs): remove after https://github.com/RiemaLabs/go-libp2p-header/issues/32 is
	//  implemented and fetch header from HeaderEx if missing locally
	head, err = s.store.Head(ctx)
	switch {
	case err != nil:
		return nil, err
	case head.Height() == height:
		return head, nil
	// `+1` allows for one header network lag, e.g. user request header that is milliseconds away
	case head.Height()+1 < height:
		return nil, fmt.Errorf("header: syncing in progress: "+
			"localHeadHeight: %d, requestedHeight: %d", head.Height(), height)
	default:
		return s.store.GetByHeight(ctx, height)
	}
}

func (s *Service) GetBtcHeight(ctx context.Context, nubitHeight uint64) (uint64, error) {
	if header, err := s.GetByHeight(ctx, nubitHeight); err != nil {
		return 0, err
	} else if data, gerr := s.edsStore.Get(ctx, header.DAH.Hash()); gerr != nil {
		return 0, gerr
	} else {
		return app.GetBtcHeightFromTx(data.GetBtcHeightTx())
	}
}

func (s *Service) GetHeightRangeAtBtcHeight(ctx context.Context, btcHeight uint64) ([]*header.ExtendedHeader, error) {
	latestHeight := s.store.Height()
	getBtcHeightFunc := func(h uint64) (uint64, error) {
		return s.GetBtcHeight(ctx, h)
	}
	heightRange, err := FindHeightsByBtcHeight(getBtcHeightFunc, btcHeight, latestHeight)
	if err != nil || len(heightRange) == 0 {
		return nil, err
	}
	from, err := s.GetByHeight(ctx, heightRange[0])
	if err != nil {
		return nil, err
	}
	if len(heightRange) == 1 {
		return []*header.ExtendedHeader{from}, nil
	}
	return s.store.GetRangeByHeight(ctx, from, heightRange[len(heightRange)-1])
}

func (s *Service) WaitForHeight(ctx context.Context, height uint64) (*header.ExtendedHeader, error) {
	return s.store.GetByHeight(ctx, height)
}

func (s *Service) LocalHead(ctx context.Context) (*header.ExtendedHeader, error) {
	return s.store.Head(ctx)
}

func (s *Service) SyncState(context.Context) (sync.State, error) {
	return s.syncer.State(), nil
}

func (s *Service) SyncWait(ctx context.Context) error {
	return s.syncer.SyncWait(ctx)
}

func (s *Service) NetworkHead(ctx context.Context) (*header.ExtendedHeader, error) {
	return s.syncer.Head(ctx)
}

func (s *Service) Subscribe(ctx context.Context) (<-chan *header.ExtendedHeader, error) {
	subscription, err := s.sub.Subscribe()
	if err != nil {
		return nil, err
	}

	headerCh := make(chan *header.ExtendedHeader)
	go func() {
		defer close(headerCh)
		defer subscription.Cancel()

		for {
			h, err := subscription.NextHeader(ctx)
			if err != nil {
				if err != context.DeadlineExceeded && err != context.Canceled {
					log.Errorw("fetching header from subscription", "err", err)
				}
				return
			}

			select {
			case <-ctx.Done():
				return
			case headerCh <- h:
			}
		}
	}()
	return headerCh, nil
}

func FindHeightsByBtcHeight(getBtcHeightFunc func(h uint64) (uint64, error), btcHeight uint64, latestHeight uint64) ([]uint64, error) {
	return findRange(getBtcHeightFunc, latestHeight, float64(btcHeight))
}

func findRange(getBtcHeightFunc func(h uint64) (uint64, error), maxIndex uint64, height float64) ([]uint64, error) {
	left, lerr := binarySearch(getBtcHeightFunc, maxIndex, height-0.5)
	if lerr != nil {
		return nil, lerr
	}
	right, rerr := binarySearch(getBtcHeightFunc, maxIndex, height+0.5)
	if rerr != nil {
		return nil, rerr
	}
	if left >= right {
		return nil, fmt.Errorf("header111: failed to find range for btcHeight %f", height)
	}
	var r []uint64
	for i := left; i < right; i++ {
		r = append(r, i)
	}
	return r, nil
}

func binarySearch(getBtcHeightFunc func(h uint64) (uint64, error), maxIndex uint64, target float64) (uint64, error) {
	left, right := uint64(1), maxIndex
	for left < right {
		mid := left + (right-left)/2
		btcHeight, err := getBtcHeightFunc(mid)
		//if err != nil {
		//	return 0, errors.New(err.Error() + "height" + fmt.Sprint(mid))
		//}
		if err != nil || float64(btcHeight) < target {
			left = uint64(mid) + 1
		} else {
			right = mid
		}
	}
	return left, nil
}

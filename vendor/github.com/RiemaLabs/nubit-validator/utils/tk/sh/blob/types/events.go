package types

import (
	"github.com/cosmos/gogoproto/proto"
)

var EventTypeSubmitBlobPayment = proto.MessageName(&EventSubmitBlobPayments{})

// NewSubmitBlobPaymentsEvent returns a new EventSubmitBlobPayments
func NewSubmitBlobPaymentsEvent(signer string, blobSizes []uint32, namespaces [][]byte) *EventSubmitBlobPayments {
	return &EventSubmitBlobPayments{
		Signer:     signer,
		BlobSizes:  blobSizes,
		Namespaces: namespaces,
	}
}

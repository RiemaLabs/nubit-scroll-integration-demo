package ipld

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"hash"
	"math/rand"

	"github.com/ipfs/boxo/blockservice"
	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	ipld "github.com/ipfs/go-ipld-format"
	logging "github.com/ipfs/go-log/v2"
	mh "github.com/multiformats/go-multihash"
	mhcore "github.com/multiformats/go-multihash/core"

	"github.com/RiemaLabs/nubit-validator/da/da"
	"github.com/RiemaLabs/nubit-validator/utils/appconsts"
	kzg "github.com/RiemaLabs/nubit-kzg"

	share "github.com/RiemaLabs/nubit-node/da"
)

var (
	log = logging.Logger("ipld")
)

const (
	// Below used multiformats (one codec, one multihash) seem free:
	// https://github.com/multiformats/multicodec/blob/master/table.csv

	// nmtCodec is the codec used for leaf and inner nodes of a Namespaced Merkle Tree.
	nmtCodec = 0x7700

	// sha256NamespaceFlagged is the multihash code used to hash blocks
	// that contain an NMT node (inner and leaf nodes).
	sha256NamespaceFlagged = 0x7701

	kzgNamespaceFlagged = 0x7705

	// NmtHashSize is the size of a digest created by an NMT in bytes.
	//NmtHashSize = 2*share.NamespaceSize + sha256.Size
	NmtHashSize = sha256.Size

	// sha256NamespaceFlagged is the multihash code used to hash hashleaf
	kzgCommitmendFlagged = 0x7709

	kzgCommitmenSize = kzg.CommitmentSize

	NkzgCommitmentSize = 2*share.NamespaceSize + kzg.CommitmentSize

	// leafNodeSize is the size of data in leaf nodes.
	leafNodeSize = share.NamespaceSize + appconsts.ShareSize

	// cidPrefixSize is the size of the prepended buffer of the CID encoding
	// for NamespacedSha256. For more information, see:
	// https://multiformats.io/multihash/#the-multihash-format
	cidPrefixSize = 4

	// NMTIgnoreMaxNamespace is currently used value for IgnoreMaxNamespace option in NMT.
	// IgnoreMaxNamespace defines whether the largest possible Namespace MAX_NID should be 'ignored'.
	// If set to true, this allows for shorter proofs in particular use-cases.
	NMTIgnoreMaxNamespace = true
)

func init() {
	// required for Bitswap to hash and verify inbound data correctly
	mhcore.Register(sha256NamespaceFlagged, func() hash.Hash {
		nh := kzg.NewNmtHasher(sha256.New(), share.NamespaceSize /*, true*/)
		nh.Reset()
		return nh
	})

	mhcore.Register(kzgCommitmendFlagged, func() hash.Hash {
		nh := kzg.NewKzgHasher()
		nh.Reset()
		return nh
	})
}

func GetNode(ctx context.Context, bGetter blockservice.BlockGetter, root cid.Cid) (ipld.Node, error) {
	block, err := bGetter.GetBlock(ctx, root)
	// fmt.Printf("🥉🥉 %s, err: %v,  res: %v\n", root.String(), err, block)
	if err != nil {
		var errNotFound ipld.ErrNotFound
		if errors.As(err, &errNotFound) {
			return nil, ErrNodeNotFound
		}
		return nil, err
	}

	return nmtNode{Block: block}, nil
}

type nmtNode struct {
	blocks.Block
}

func newNMTNode(id cid.Cid, data []byte) nmtNode {
	b, err := blocks.NewBlockWithCid(data, id)
	if err != nil {
		panic(fmt.Sprintf("wrong hash for block, cid: %s", id.String()))
	}
	return nmtNode{Block: b}
}

func (n nmtNode) Copy() ipld.Node {
	d := make([]byte, len(n.RawData()))
	copy(d, n.RawData())
	return newNMTNode(n.Cid(), d)
}

func (n nmtNode) Links() []*ipld.Link {
	dataSize := len(n.RawData())

	switch {
	case dataSize == leafNodeSize:
		return nil
	case dataSize%NmtHashSize == 0:
		links := make([]*ipld.Link, 0, dataSize/NmtHashSize)
		for i := 0; i < dataSize; i += NmtHashSize {
			links = append(links, &ipld.Link{Cid: MustCidFromNamespacedSha256(n.RawData()[i : i+NmtHashSize])})
		}
		return links
	default:
		panic(fmt.Sprintf("unexpected size %v", len(n.RawData())))
	}
}

func (n nmtNode) Resolve([]string) (interface{}, []string, error) {
	panic("method not implemented")
}

func (n nmtNode) Tree(string, int) []string {
	panic("method not implemented")
}

func (n nmtNode) ResolveLink([]string) (*ipld.Link, []string, error) {
	panic("method not implemented")
}

func (n nmtNode) Stat() (*ipld.NodeStat, error) {
	panic("method not implemented")
}

func (n nmtNode) Size() (uint64, error) {
	panic("method not implemented")
}

// CidFromNamespacedSha256 uses a hash from an nmt tree to create a CID
func CidFromNamespacedSha256(namespacedHash []byte) (cid.Cid, error) {
	if got, want := len(namespacedHash), NmtHashSize; got != want {
		return cid.Cid{}, fmt.Errorf("invalid namespaced hash length, got: %v, want: %v", got, want)
	}
	buf, err := mh.Encode(namespacedHash, sha256NamespaceFlagged)
	if err != nil {
		return cid.Undef, err
	}
	return cid.NewCidV1(nmtCodec, buf), nil
}

// MustCidFromNamespacedSha256 is a wrapper around cidFromNamespacedSha256 that panics
// in case of an error. Use with care and only in places where no error should occur.
func MustCidFromNamespacedSha256(hash []byte) cid.Cid {
	cidFromHash, err := CidFromNamespacedSha256(hash)
	if err != nil {
		panic(
			fmt.Sprintf("malformed hash: %s, codec: %v",
				err,
				mh.Codes[sha256NamespaceFlagged]),
		)
	}
	return cidFromHash
}

func CidFromNamespacedKzg(namespacedHash []byte) (cid.Cid, error) {
	if got, want := len(namespacedHash), NkzgCommitmentSize; got != want {
		return cid.Cid{}, fmt.Errorf("invalid namespaced kzg hash length, got: %v, want: %v", got, want)
	}
	buf, err := mh.Encode(namespacedHash, kzgNamespaceFlagged)
	if err != nil {
		return cid.Undef, err
	}
	return cid.NewCidV1(nmtCodec, buf), nil
}

func MustCidFromNamespacedKzg(hash []byte) cid.Cid {
	cidFromHash, err := CidFromNamespacedKzg(hash)
	if err != nil {
		panic(
			fmt.Sprintf("malformed hash: %s, codec: %v",
				err,
				mh.Codes[kzgNamespaceFlagged]),
		)
	}
	return cidFromHash
}

func CidFromKzg(namespacedHash []byte) (cid.Cid, error) {
	if got, want := len(namespacedHash), kzgCommitmenSize; got != want {
		return cid.Cid{}, fmt.Errorf("invalid kzg commit length, got: %v, want: %v", got, want)
	}
	buf, err := mh.Encode(namespacedHash, kzgCommitmendFlagged)
	if err != nil {
		return cid.Undef, err
	}
	return cid.NewCidV1(nmtCodec, buf), nil
}

func MustCidFromKzg(hash []byte) cid.Cid {
	cidFromHash, err := CidFromKzg(hash)
	if err != nil {
		panic(
			fmt.Sprintf("malformed hash: %s, codec: %v",
				err,
				mh.Codes[kzgCommitmendFlagged]),
		)
	}
	return cidFromHash
}

// Translate transforms square coordinates into IPLD NMT tree path to a leaf node.
// It also adds randomization to evenly spread fetching from Rows and Columns.
func Translate(dah *da.DataAvailabilityHeader, row, col int) (cid.Cid, int) {

	if rand.Intn(2) == 0 { //nolint:gosec
		r := dah.RowRoots[row]
		return MustCidFromKzg(r[2*share.NamespaceSize:]), row
	}
	r := dah.ColumnRoots[col]
	return MustCidFromKzg(r[2*share.NamespaceSize:]), col
}

/*
func TranslateToCom(dah *da.DataAvailabilityHeader, row, col int) (cid.Cid, int) {
	if rand.Intn(2) == 0 { //nolint:gosec
		return MustCidFromNamespacedSha256(dah.ColumnCommitments[col]), row
	}

	return MustCidFromNamespacedSha256(dah.RowCommitments[row]), col
}
*/

// NamespacedSha256FromCID derives the Namespaced hash from the given CID.
func NamespacedSha256FromCID(cid cid.Cid) []byte {
	return cid.Hash()[cidPrefixSize:]
}

func NamespacedKzgFromCID(cid cid.Cid) []byte {
	return cid.Hash()[cidPrefixSize:]
}

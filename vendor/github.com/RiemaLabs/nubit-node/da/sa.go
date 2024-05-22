package share

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"

	"crypto/sha256"

	"github.com/RiemaLabs/nubit-validator/da/da"
	"github.com/RiemaLabs/nubit-validator/da/shares"
	"github.com/RiemaLabs/nubit-validator/utils/appconsts"
	kzg "github.com/RiemaLabs/nubit-kzg"
	header "github.com/RiemaLabs/nubit-node/strucs/eh"
	"github.com/RiemaLabs/rsmt2d"
)

var (
	// DefaultRSMT2DCodec sets the default rsmt2d.Codec for shares.
	DefaultRSMT2DCodec = appconsts.DefaultCodec
)

const (
	// Size is a system-wide size of a share, including both data and namespace GetNamespace
	Size = appconsts.ShareSize
)

var (
	// MaxSquareSize is currently the maximum size supported for unerasured data in
	// rsmt2d.ExtendedDataSquare.
	MaxSquareSize = appconsts.SquareSizeUpperBound(appconsts.LatestVersion)
	// ErrNotFound is used to indicate that requested data could not be found.
	ErrNotFound = errors.New("share: data not found")
	// ErrOutOfBounds is used to indicate that a passed row or column index is out of bounds of the
	// square size.
	ErrOutOfBounds = errors.New("share: row or column index is larger than square size")
)

// Share contains the raw share data without the corresponding namespace.
// NOTE: Alias for the byte is chosen to keep maximal compatibility, especially with rsmt2d.
// Ideally, we should define reusable type elsewhere and make everyone(Core, rsmt2d, ipld) to rely
// on it.
type Share = []byte

// GetNamespace slices Namespace out of the Share.
func GetNamespace(s Share) Namespace {
	return s[:NamespaceSize]
}

// GetData slices out data of the Share.
func GetData(s Share) []byte {
	return s[NamespaceSize:]
}

// DataHash is a representation of the Root hash.
type DataHash []byte

func (dh DataHash) Validate() error {
	if len(dh) != 32 {
		return fmt.Errorf("invalid hash size, expected 32, got %d", len(dh))
	}
	return nil
}

func (dh DataHash) String() string {
	return fmt.Sprintf("%X", []byte(dh))
}

// IsEmptyRoot check whether DataHash corresponds to the root of an empty block EDS.
func (dh DataHash) IsEmptyRoot() bool {
	return bytes.Equal(EmptyRoot().Hash(), dh)
}

// MustDataHashFromString converts a hex string to a valid datahash.
func MustDataHashFromString(datahash string) DataHash {
	dh, err := hex.DecodeString(datahash)
	if err != nil {
		panic(fmt.Sprintf("datahash conversion: passed string was not valid hex: %s", datahash))
	}
	err = DataHash(dh).Validate()
	if err != nil {
		panic(fmt.Sprintf("datahash validation: passed hex string failed: %s", err))
	}
	return dh
}

// ErrNotAvailable is returned whenever DA sampling fails.
var ErrNotAvailable = errors.New("share: data not available")

// Root represents root commitment to multiple Shares.
// In practice, it is a commitment to all the Data in a square.
type Root = da.DataAvailabilityHeader

// NewRoot generates Root(DataAvailabilityHeader) using the
// provided extended data square.
func NewRoot(eds *rsmt2d.ExtendedDataSquare) (*Root, error) {
	dah, err := da.NewDataAvailabilityHeader(eds)
	if err != nil {
		return nil, err
	}
	return &dah, nil
}

// Availability defines interface for validation of Shares' availability.
//
//go:generate mockgen -destination=availability/mocks/availability.go -package=mocks . Availability
type Availability interface {
	// SharesAvailable subjectively validates if Shares committed to the given Root are available on
	// the Network.
	SharesAvailable(context.Context, *header.ExtendedHeader) error
}

type Getter interface {
	// GetShare gets a Share by coordinates in EDS.
	GetShare(ctx context.Context, header *header.ExtendedHeader, row, col int) (Share, error)

	// GetEDS gets the full EDS identified by the given extended header.
	GetEDS(context.Context, *header.ExtendedHeader) (*rsmt2d.ExtendedDataSquare, error)

	// GetSharesByNamespace gets all shares from an EDS within the given namespace.
	// Shares are returned in a row-by-row order if the namespace spans multiple rows.
	// Inclusion of returned data could be verified using Verify method on NamespacedShares.
	// If no shares are found for target namespace non-inclusion could be also verified by calling
	// Verify method.
	GetSharesByNamespace(context.Context, *header.ExtendedHeader, Namespace) (NamespacedShares, error)
}

// NamespacedShares represents all shares with proofs within a specific namespace of an EDS.
type NamespacedShares []NamespacedRow

// Flatten returns the concatenated slice of all NamespacedRow shares.
func (ns NamespacedShares) Flatten() []Share {
	shares := make([]Share, 0)
	for _, row := range ns {
		shares = append(shares, row.Shares...)
	}
	return shares
}

// NamespacedRow represents all shares with proofs within a specific namespace of a single EDS row.
type NamespacedRow struct {
	Shares []Share                  `json:"shares"`
	Proof  *kzg.NamespaceRangeProof `json:"proof"`
}

// Verify validates NamespacedShares by checking every row with nmt inclusion proof.
func (ns NamespacedShares) Verify(root *Root, namespace Namespace) error {
	var originalRoots [][]byte
	for _, row := range root.RowRoots {
		if !namespace.IsOutsideRange(row, row) {
			originalRoots = append(originalRoots, row)
		}
	}

	if len(originalRoots) != len(ns) {
		return fmt.Errorf("amount of rows differs between root and namespace shares: expected %d, got %d",
			len(originalRoots), len(ns))
	}

	for i, row := range ns {
		// verify row data against row hash from original root
		if !row.verify(originalRoots[i], namespace) {
			return fmt.Errorf("row verification failed: row %d doesn't match original root: %s", i, root.String())
		}
	}
	return nil
}

// verify validates the row using nmt inclusion proof.
func (row *NamespacedRow) verify(rowRoot []byte, namespace Namespace) bool {
	// construct nmt leaves from shares by prepending namespace
	leaves := make([][]byte, 0, len(row.Shares))
	for _, shr := range row.Shares {
		leaves = append(leaves, append(GetNamespace(shr), shr...))
	}

	// verify namespace
	return row.Proof.VerifyNamespace(
		sha256.New(),
		namespace.ToNMT(),
		//leaves,
		rowRoot,
	)
}

// EmptyRoot returns Root of the empty block EDS.
func EmptyRoot() *Root {
	initEmpty()
	return emptyBlockRoot
}

// EmptyExtendedDataSquare returns the EDS of the empty block data square.
func EmptyExtendedDataSquare() *rsmt2d.ExtendedDataSquare {
	initEmpty()
	return emptyBlockEDS
}

// EmptyBlockShares returns the shares of the empty block.
func EmptyBlockShares() []Share {
	initEmpty()
	return emptyBlockShares
}

var (
	emptyMu          sync.Mutex
	emptyBlockRoot   *Root
	emptyBlockEDS    *rsmt2d.ExtendedDataSquare
	emptyBlockShares []Share
)

// initEmpty enables lazy initialization for constant empty block data.
func initEmpty() {
	emptyMu.Lock()
	defer emptyMu.Unlock()
	if emptyBlockRoot != nil {
		return
	}

	// compute empty block EDS and DAH for it
	result := shares.TailPaddingShares(appconsts.MinShareCount)
	emptyBlockShares = shares.ToBytes(result)

	eds, err := da.ExtendShares(emptyBlockShares)
	if err != nil {
		panic(fmt.Errorf("failed to create empty EDS: %w", err))
	}
	emptyBlockEDS = eds

	emptyBlockRoot, err = NewRoot(eds)
	if err != nil {
		panic(fmt.Errorf("failed to create empty DAH: %w", err))
	}
	minDAH := da.MinDataAvailabilityHeader()
	if !bytes.Equal(minDAH.Hash(), emptyBlockRoot.Hash()) {
		panic(fmt.Sprintf("mismatch in calculated minimum DAH and minimum DAH from nubit-validator, "+
			"expected %s, got %s", minDAH.String(), emptyBlockRoot.String()))
	}

	// precompute Hash, so it's cached internally to avoid potential races
	emptyBlockRoot.Hash()
}

// HashRoot represents hash(root commitment to multiple Shares).
type HashRoot struct {
	RowRoots    [][]byte
	ColumnRoots [][]byte
}

// NewRoot generates Root(DataAvailabilityHeader) using the
// provided extended data square.
func NewHashRoot(eds *rsmt2d.ExtendedDataSquare) (*HashRoot, error) {
	dah, err := da.NewDataAvailabilityHeader(eds)
	if err != nil {
		return nil, err
	}

	hasher := kzg.NewNmtHasher(sha256.New(), NamespaceSize)
	hashRoot := HashRoot{
		RowRoots:    make([][]byte, len(dah.RowRoots)),
		ColumnRoots: make([][]byte, len(dah.ColumnRoots)),
	}
	for i, r := range dah.RowRoots {
		m := hasher.MustHashLeaf(r)
		copy(hashRoot.RowRoots[i], m)
	}
	for i, r := range dah.ColumnRoots {
		m := hasher.MustHashLeaf(r)
		copy(hashRoot.ColumnRoots[i], m)
	}

	return &hashRoot, nil
}

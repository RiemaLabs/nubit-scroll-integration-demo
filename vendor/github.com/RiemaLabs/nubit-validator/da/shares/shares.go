package shares

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"

	appns "github.com/RiemaLabs/nubit-validator/da/namespace"
	"github.com/RiemaLabs/nubit-validator/utils/appconsts"
	coretypes "github.com/tendermint/tendermint/types"
	"golang.org/x/exp/maps"
)

// Share contains the raw share data (including namespace ID).
type Share struct {
	data []byte
}

func (s *Share) Namespace() (appns.Namespace, error) {
	if len(s.data) < appns.NamespaceSize {
		panic(fmt.Sprintf("share %s is too short to contain a namespace", s))
	}
	return appns.From(s.data[:appns.NamespaceSize])
}

func (s *Share) InfoByte() (InfoByte, error) {
	if len(s.data) < appns.NamespaceSize+appconsts.ShareInfoBytes {
		return 0, fmt.Errorf("share %s is too short to contain an info byte", s)
	}
	// the info byte is the first byte after the namespace
	unparsed := s.data[appns.NamespaceSize]
	return ParseInfoByte(unparsed)
}

func NewShare(data []byte) (*Share, error) {
	if err := validateSize(data); err != nil {
		return nil, err
	}
	return &Share{data}, nil
}

func (s *Share) Validate() error {
	return validateSize(s.data)
}

func validateSize(data []byte) error {
	if len(data) != appconsts.ShareSize {
		return fmt.Errorf("share data must be %d bytes, got %d", appconsts.ShareSize, len(data))
	}
	return nil
}

func (s *Share) Len() int {
	return len(s.data)
}

func (s *Share) Version() (uint8, error) {
	infoByte, err := s.InfoByte()
	if err != nil {
		return 0, err
	}
	return infoByte.Version(), nil
}

func (s *Share) DoesSupportVersions(supportedShareVersions []uint8) error {
	ver, err := s.Version()
	if err != nil {
		return err
	}
	if !bytes.Contains(supportedShareVersions, []byte{ver}) {
		return fmt.Errorf("unsupported share version %v is not present in the list of supported share versions %v", ver, supportedShareVersions)
	}
	return nil
}

// IsSequenceStart returns true if this is the first share in a sequence.
func (s *Share) IsSequenceStart() (bool, error) {
	infoByte, err := s.InfoByte()
	if err != nil {
		return false, err
	}
	return infoByte.IsSequenceStart(), nil
}

// IsCompactShare returns true if this is a compact share.
func (s Share) IsCompactShare() (bool, error) {
	ns, err := s.Namespace()
	if err != nil {
		return false, err
	}
	isCompact := ns.IsTx() || ns.IsSubmitBlobPayment()
	return isCompact, nil
}

// SequenceLen returns the sequence length of this *share and optionally an
// error. It returns 0, nil if this is a continuation share (i.e. doesn't
// contain a sequence length).
func (s *Share) SequenceLen() (sequenceLen uint32, err error) {
	isSequenceStart, err := s.IsSequenceStart()
	if err != nil {
		return 0, err
	}
	if !isSequenceStart {
		return 0, nil
	}

	start := appconsts.NamespaceSize + appconsts.ShareInfoBytes
	end := start + appconsts.SequenceLenBytes
	if len(s.data) < end {
		return 0, fmt.Errorf("share %s with length %d is too short to contain a sequence length",
			s, len(s.data))
	}
	return binary.BigEndian.Uint32(s.data[start:end]), nil
}

// IsPadding returns whether this *share is padding or not.
func (s *Share) IsPadding() (bool, error) {
	isNamespacePadding, err := s.isNamespacePadding()
	if err != nil {
		return false, err
	}
	isTailPadding, err := s.isTailPadding()
	if err != nil {
		return false, err
	}
	isPrimaryReservedPadding, err := s.isPrimaryReservedPadding()
	if err != nil {
		return false, err
	}
	return isNamespacePadding || isTailPadding || isPrimaryReservedPadding, nil
}

func (s *Share) isNamespacePadding() (bool, error) {
	isSequenceStart, err := s.IsSequenceStart()
	if err != nil {
		return false, err
	}
	sequenceLen, err := s.SequenceLen()
	if err != nil {
		return false, err
	}

	return isSequenceStart && sequenceLen == 0, nil
}

func (s *Share) isTailPadding() (bool, error) {
	ns, err := s.Namespace()
	if err != nil {
		return false, err
	}
	return ns.IsTailPadding(), nil
}

func (s *Share) isPrimaryReservedPadding() (bool, error) {
	ns, err := s.Namespace()
	if err != nil {
		return false, err
	}
	return ns.IsPrimaryReservedPadding(), nil
}

func (s *Share) ToBytes() []byte {
	return s.data
}

// RawData returns the raw share data. The raw share data does not contain the
// namespace ID, info byte, sequence length, or reserved bytes.
func (s *Share) RawData() (rawData []byte, err error) {
	if len(s.data) < s.rawDataStartIndex() {
		return rawData, fmt.Errorf("share %s is too short to contain raw data", s)
	}

	return s.data[s.rawDataStartIndex():], nil
}

func (s *Share) rawDataStartIndex() int {
	isStart, err := s.IsSequenceStart()
	if err != nil {
		panic(err)
	}
	isCompact, err := s.IsCompactShare()
	if err != nil {
		panic(err)
	}

	index := appconsts.NamespaceSize + appconsts.ShareInfoBytes
	if isStart {
		index += appconsts.SequenceLenBytes
	}
	if isCompact {
		index += appconsts.CompactShareReservedBytes
	}
	return index
}

// RawDataWithReserved returns the raw share data while taking reserved bytes into account.
func (s *Share) RawDataUsingReserved() (rawData []byte, err error) {
	rawDataStartIndexUsingReserved, err := s.rawDataStartIndexUsingReserved()
	if err != nil {
		return nil, err
	}

	// This means share is the last share and does not have any transaction beginning in it
	if rawDataStartIndexUsingReserved == 0 {
		return []byte{}, nil
	}
	if len(s.data) < rawDataStartIndexUsingReserved {
		return rawData, fmt.Errorf("share %s is too short to contain raw data", s)
	}

	return s.data[rawDataStartIndexUsingReserved:], nil
}

// rawDataStartIndexUsingReserved returns the start index of raw data while accounting for
// reserved bytes, if it exists in the share.
func (s *Share) rawDataStartIndexUsingReserved() (int, error) {
	isStart, err := s.IsSequenceStart()
	if err != nil {
		return 0, err
	}
	isCompact, err := s.IsCompactShare()
	if err != nil {
		return 0, err
	}

	index := appconsts.NamespaceSize + appconsts.ShareInfoBytes
	if isStart {
		index += appconsts.SequenceLenBytes
	}

	if isCompact {
		reservedBytes, err := ParseReservedBytes(s.data[index : index+appconsts.CompactShareReservedBytes])
		if err != nil {
			return 0, err
		}
		return int(reservedBytes), nil
	}
	return index, nil
}

func ToBytes(shares []Share) (bytes [][]byte) {
	bytes = make([][]byte, len(shares))
	for i, share := range shares {
		bytes[i] = []byte(share.data)
	}
	return bytes
}

func FromBytes(bytes [][]byte) (shares []Share, err error) {
	for _, b := range bytes {
		share, err := NewShare(b)
		if err != nil {
			return nil, err
		}
		shares = append(shares, *share)
	}
	return shares, nil
}

var (
	ErrIncorrectNumberOfIndexes = errors.New(
		"number of indexes is not identical to the number of blobs",
	)
	ErrUnexpectedFirstBlobShareIndex = errors.New(
		"the first blob started at an unexpected index",
	)
)

// ExtractShareIndexes iterates over the transactions and extracts the share
// indexes from wrapped transactions. It returns nil if the transactions are
// from an old block that did not have share indexes in the wrapped txs.
func ExtractShareIndexes(txs coretypes.Txs) []uint32 {
	var shareIndexes []uint32
	for _, rawTx := range txs {
		if indexWrappedTxs, isIndexWrapped := coretypes.UnmarshalIndexWrapper(rawTx); isIndexWrapped {
			// Since share index == 0 is invalid, it indicates that we are
			// attempting to extract share indexes from txs that do not have any
			// due to them being old. here we return nil to indicate that we are
			// attempting to extract indexes from a block that doesn't support
			// it. It checks for 0 because if there is a message in the block,
			// then there must also be a tx, which will take up at least one
			// share.
			if len(indexWrappedTxs.ShareIndexes) == 0 {
				return nil
			}
			shareIndexes = append(shareIndexes, indexWrappedTxs.ShareIndexes...)
		}
	}

	return shareIndexes
}

func SplitTxs(txs coretypes.Txs) (txShares []Share, pfbShares []Share, shareRanges map[coretypes.TxKey]Range, err error) {
	txWriter := NewCompactShareSplitter(appns.TxNamespace, appconsts.ShareVersionZero)
	pfbTxWriter := NewCompactShareSplitter(appns.SubmitBlobPaymentNamespace, appconsts.ShareVersionZero)

	for _, tx := range txs {
		if _, isIndexWrapper := coretypes.UnmarshalIndexWrapper(tx); isIndexWrapper {
			err = pfbTxWriter.WriteTx(tx)
		} else {
			err = txWriter.WriteTx(tx)
		}
		if err != nil {
			return nil, nil, nil, err
		}
	}

	txShares, err = txWriter.Export()
	if err != nil {
		return nil, nil, nil, err
	}
	txMap := txWriter.ShareRanges(0)

	pfbShares, err = pfbTxWriter.Export()
	if err != nil {
		return nil, nil, nil, err
	}
	pfbMap := pfbTxWriter.ShareRanges(len(txShares))

	return txShares, pfbShares, mergeMaps(txMap, pfbMap), nil
}

// SplitBlobs splits the provided blobs into shares.
func SplitBlobs(blobs ...coretypes.Blob) ([]Share, error) {
	writer := NewSparseShareSplitter()
	for _, blob := range blobs {
		if err := writer.Write(blob); err != nil {
			return nil, err
		}
	}
	return writer.Export(), nil
}

// mergeMaps merges two maps into a new map. If there are any duplicate keys,
// the value in the second map takes precedence.
func mergeMaps(mapOne, mapTwo map[coretypes.TxKey]Range) map[coretypes.TxKey]Range {
	merged := make(map[coretypes.TxKey]Range, len(mapOne)+len(mapTwo))
	maps.Copy(merged, mapOne)
	maps.Copy(merged, mapTwo)
	return merged
}

// ShareSequence represents a contiguous sequence of shares that are part of the
// same namespace and blob. For compact shares, one share sequence exists per
// reserved namespace. For sparse shares, one share sequence exists per blob.
type ShareSequence struct {
	Namespace appns.Namespace
	Shares    []Share
}

// RawData returns the raw share data of this share sequence. The raw data does
// not contain the namespace ID, info byte, sequence length, or reserved bytes.
func (s ShareSequence) RawData() (data []byte, err error) {
	for _, share := range s.Shares {
		raw, err := share.RawData()
		if err != nil {
			return []byte{}, err
		}
		data = append(data, raw...)
	}

	sequenceLen, err := s.SequenceLen()
	if err != nil {
		return []byte{}, err
	}
	// trim any padding that may have been added to the last share
	return data[:sequenceLen], nil
}

func (s ShareSequence) SequenceLen() (uint32, error) {
	if len(s.Shares) == 0 {
		return 0, fmt.Errorf("invalid sequence length because share sequence %v has no shares", s)
	}
	firstShare := s.Shares[0]
	return firstShare.SequenceLen()
}

// validSequenceLen extracts the sequenceLen written to the first share
// and returns an error if the number of shares needed to store a sequence of
// length sequenceLen doesn't match the number of shares in this share
// sequence. Returns nil if there is no error.
func (s ShareSequence) validSequenceLen() error {
	if len(s.Shares) == 0 {
		return fmt.Errorf("invalid sequence length because share sequence %v has no shares", s)
	}
	isPadding, err := s.isPadding()
	if err != nil {
		return err
	}
	if isPadding {
		return nil
	}

	firstShare := s.Shares[0]
	sharesNeeded, err := numberOfSharesNeeded(firstShare)
	if err != nil {
		return err
	}

	if len(s.Shares) != sharesNeeded {
		return fmt.Errorf("share sequence has %d shares but needed %d shares", len(s.Shares), sharesNeeded)
	}
	return nil
}

func (s ShareSequence) isPadding() (bool, error) {
	if len(s.Shares) != 1 {
		return false, nil
	}
	isPadding, err := s.Shares[0].IsPadding()
	if err != nil {
		return false, err
	}
	return isPadding, nil
}

// numberOfSharesNeeded extracts the sequenceLen written to the share
// firstShare and returns the number of shares needed to store a sequence of
// that length.
func numberOfSharesNeeded(firstShare Share) (sharesUsed int, err error) {
	sequenceLen, err := firstShare.SequenceLen()
	if err != nil {
		return 0, err
	}

	isCompact, err := firstShare.IsCompactShare()
	if err != nil {
		return 0, err
	}
	if isCompact {
		return CompactSharesNeeded(int(sequenceLen)), nil
	}
	return SparseSharesNeeded(sequenceLen), nil
}

// CompactSharesNeeded returns the number of compact shares needed to store a
// sequence of length sequenceLen. The parameter sequenceLen is the number
// of bytes of transactions or intermediate state roots in a sequence.
func CompactSharesNeeded(sequenceLen int) (sharesNeeded int) {
	if sequenceLen == 0 {
		return 0
	}

	if sequenceLen < appconsts.FirstCompactShareContentSize {
		return 1
	}

	bytesAvailable := appconsts.FirstCompactShareContentSize
	sharesNeeded++
	for bytesAvailable < sequenceLen {
		bytesAvailable += appconsts.ContinuationCompactShareContentSize
		sharesNeeded++
	}
	return sharesNeeded
}

// SparseSharesNeeded returns the number of shares needed to store a sequence of
// length sequenceLen.
func SparseSharesNeeded(sequenceLen uint32) (sharesNeeded int) {
	if sequenceLen == 0 {
		return 0
	}

	if sequenceLen < appconsts.FirstSparseShareContentSize {
		return 1
	}

	bytesAvailable := appconsts.FirstSparseShareContentSize
	sharesNeeded++
	for uint32(bytesAvailable) < sequenceLen {
		bytesAvailable += appconsts.ContinuationSparseShareContentSize
		sharesNeeded++
	}
	return sharesNeeded
}

package shares

import (
	"bytes"
	"fmt"

	"github.com/RiemaLabs/nubit-validator/utils/appconsts"
	coretypes "github.com/tendermint/tendermint/types"
)

// ParseTxs collects all of the transactions from the shares provided
func ParseTxs(shares []Share) (coretypes.Txs, error) {
	// parse the shares
	rawTxs, err := parseCompactShares(shares, appconsts.SupportedShareVersions)
	if err != nil {
		return nil, err
	}

	// convert to the Tx type
	txs := make(coretypes.Txs, len(rawTxs))
	for i := 0; i < len(txs); i++ {
		txs[i] = coretypes.Tx(rawTxs[i])
	}

	return txs, nil
}

// ParseBlobs collects all blobs from the shares provided
func ParseBlobs(shares []Share) ([]coretypes.Blob, error) {
	blobList, err := parseSparseShares(shares, appconsts.SupportedShareVersions)
	if err != nil {
		return []coretypes.Blob{}, err
	}

	return blobList, nil
}

// ParseShares parses the shares provided and returns a list of ShareSequences.
// If ignorePadding is true then the returned ShareSequences will not contain
// any padding sequences.
func ParseShares(shares []Share, ignorePadding bool) ([]ShareSequence, error) {
	sequences := []ShareSequence{}
	currentSequence := ShareSequence{}

	for _, share := range shares {
		if err := share.Validate(); err != nil {
			return sequences, err
		}
		isStart, err := share.IsSequenceStart()
		if err != nil {
			return sequences, err
		}
		ns, err := share.Namespace()
		if err != nil {
			return sequences, err
		}
		if isStart {
			if len(currentSequence.Shares) > 0 {
				sequences = append(sequences, currentSequence)
			}
			currentSequence = ShareSequence{
				Shares:    []Share{share},
				Namespace: ns,
			}
		} else {
			if !bytes.Equal(currentSequence.Namespace.Bytes(), ns.Bytes()) {
				return sequences, fmt.Errorf("share sequence %v has inconsistent namespace IDs with share %v", currentSequence, share)
			}
			currentSequence.Shares = append(currentSequence.Shares, share)
		}
	}

	if len(currentSequence.Shares) > 0 {
		sequences = append(sequences, currentSequence)
	}

	for _, sequence := range sequences {
		if err := sequence.validSequenceLen(); err != nil {
			return sequences, err
		}
	}

	result := []ShareSequence{}
	for _, sequence := range sequences {
		isPadding, err := sequence.isPadding()
		if err != nil {
			return nil, err
		}
		if ignorePadding && isPadding {
			continue
		}
		result = append(result, sequence)
	}

	return result, nil
}

type sequence struct {
	blob        coretypes.Blob
	sequenceLen uint32
}

// parseSparseShares iterates through rawShares and parses out individual
// blobs. It returns an error if a rawShare contains a share version that
// isn't present in supportedShareVersions.
func parseSparseShares(shares []Share, supportedShareVersions []uint8) (blobs []coretypes.Blob, err error) {
	if len(shares) == 0 {
		return nil, nil
	}
	sequences := make([]sequence, 0)

	for _, share := range shares {
		version, err := share.Version()
		if err != nil {
			return nil, err
		}
		if !bytes.Contains(supportedShareVersions, []byte{version}) {
			return nil, fmt.Errorf("unsupported share version %v is not present in supported share versions %v", version, supportedShareVersions)
		}

		isPadding, err := share.IsPadding()
		if err != nil {
			return nil, err
		}
		if isPadding {
			continue
		}

		isStart, err := share.IsSequenceStart()
		if err != nil {
			return nil, err
		}

		if isStart {
			sequenceLen, err := share.SequenceLen()
			if err != nil {
				return nil, err
			}
			data, err := share.RawData()
			if err != nil {
				return nil, err
			}
			ns, err := share.Namespace()
			if err != nil {
				return nil, err
			}
			blob := coretypes.Blob{
				NamespaceID:      ns.ID,
				Data:             data,
				ShareVersion:     version,
				NamespaceVersion: ns.Version,
			}
			sequences = append(sequences, sequence{
				blob:        blob,
				sequenceLen: sequenceLen,
			})
		} else { // continuation share
			if len(sequences) == 0 {
				return nil, fmt.Errorf("continuation share %v without a sequence start share", share)
			}
			prev := &sequences[len(sequences)-1]
			data, err := share.RawData()
			if err != nil {
				return nil, err
			}
			prev.blob.Data = append(prev.blob.Data, data...)
		}
	}
	for _, sequence := range sequences {
		// trim any padding from the end of the sequence
		sequence.blob.Data = sequence.blob.Data[:sequence.sequenceLen]
		blobs = append(blobs, sequence.blob)
	}

	return blobs, nil
}

// parseCompactShares returns data (transactions or intermediate state roots
// based on the contents of rawShares and supportedShareVersions. If rawShares
// contains a share with a version that isn't present in supportedShareVersions,
// an error is returned. The returned data [][]byte does not have namespaces,
// info bytes, data length delimiter, or unit length delimiters and are ready to
// be unmarshalled.
func parseCompactShares(shares []Share, supportedShareVersions []uint8) (data [][]byte, err error) {
	if len(shares) == 0 {
		return nil, nil
	}

	err = validateShareVersions(shares, supportedShareVersions)
	if err != nil {
		return nil, err
	}

	rawData, err := extractRawData(shares)
	if err != nil {
		return nil, err
	}

	data, err = parseRawData(rawData)
	if err != nil {
		return nil, err
	}

	return data, nil
}

// validateShareVersions returns an error if the shares contain a share with an
// unsupported share version. Returns nil if all shares contain supported share
// versions.
func validateShareVersions(shares []Share, supportedShareVersions []uint8) error {
	for i := 0; i < len(shares); i++ {
		if err := shares[i].DoesSupportVersions(supportedShareVersions); err != nil {
			return err
		}
	}
	return nil
}

// parseRawData returns the units (transactions, PFB transactions, intermediate
// state roots) contained in raw data by parsing the unit length delimiter
// prefixed to each unit.
func parseRawData(rawData []byte) (units [][]byte, err error) {
	units = make([][]byte, 0)
	for {
		actualData, unitLen, err := ParseDelimiter(rawData)
		if err != nil {
			return nil, err
		}
		// the rest of raw data is padding
		if unitLen == 0 {
			return units, nil
		}
		// the rest of actual data contains only part of the next transaction so
		// we stop parsing raw data
		if unitLen > uint64(len(actualData)) {
			return units, nil
		}
		rawData = actualData[unitLen:]
		units = append(units, actualData[:unitLen])
	}
}

// extractRawData returns the raw data representing complete transactions
// contained in the shares. The raw data does not contain the namespace, info
// byte, sequence length, or reserved bytes. Starts reading raw data based on
// the reserved bytes in the first share.
func extractRawData(shares []Share) (rawData []byte, err error) {
	for i := 0; i < len(shares); i++ {
		var raw []byte
		if i == 0 {
			raw, err = shares[i].RawDataUsingReserved()
		} else {
			raw, err = shares[i].RawData()
		}
		if err != nil {
			return nil, err
		}
		rawData = append(rawData, raw...)
	}
	return rawData, nil
}

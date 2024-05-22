package proof

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"

	"github.com/RiemaLabs/nubit-validator/da/shares"
	"github.com/RiemaLabs/nubit-validator/da/square"
	"github.com/RiemaLabs/nubit-validator/utils/appconsts"

	appns "github.com/RiemaLabs/nubit-validator/da/namespace"
	sdk "github.com/cosmos/cosmos-sdk/types"
	abci "github.com/tendermint/tendermint/abci/types"
	tmproto "github.com/tendermint/tendermint/proto/tendermint/types"
	"github.com/tendermint/tendermint/types"

	"github.com/RiemaLabs/nubit-validator/da/da"
	"github.com/RiemaLabs/nubit-validator/utils/wrapper"
	"github.com/tendermint/tendermint/crypto/merkle"
	tmbytes "github.com/tendermint/tendermint/libs/bytes"
)

// NewTxInclusionProof returns a new share inclusion proof for the given
// transaction index.
func NewTxInclusionProof(txs [][]byte, txIndex, appVersion uint64) (types.ShareProof, error) {
	if txIndex >= uint64(len(txs)) {
		return types.ShareProof{}, fmt.Errorf("txIndex %d out of bounds", txIndex)
	}

	builder, err := square.NewBuilder(appconsts.SquareSizeUpperBound(appVersion), appVersion, txs...)
	if err != nil {
		return types.ShareProof{}, err
	}

	dataSquare, err := builder.Export()
	if err != nil {
		return types.ShareProof{}, err
	}

	shareRange, err := builder.FindTxShareRange(int(txIndex))
	if err != nil {
		return types.ShareProof{}, err
	}

	namespace := getTxNamespace(txs[txIndex])
	return NewShareInclusionProof(dataSquare, namespace, shareRange)
}

func getTxNamespace(tx []byte) (ns appns.Namespace) {
	_, isBlobTx := types.UnmarshalBlobTx(tx)
	if isBlobTx {
		return appns.SubmitBlobPaymentNamespace
	}
	return appns.TxNamespace
}

// NewShareInclusionProof returns an NMT inclusion proof for a set of shares
// belonging to the same namespace to the data root.
// Expects the share range to be pre-validated.
func NewShareInclusionProof(
	dataSquare square.Square,
	namespace appns.Namespace,
	shareRange shares.Range,
) (types.ShareProof, error) {
	squareSize := dataSquare.Size()
	startRow := shareRange.Start / squareSize
	endRow := (shareRange.End - 1) / squareSize
	startLeaf := shareRange.Start % squareSize
	endLeaf := (shareRange.End - 1) % squareSize

	eds, err := da.ExtendShares(shares.ToBytes(dataSquare))
	if err != nil {
		return types.ShareProof{}, err
	}

	edsRowRoots, err := eds.RowRoots()
	if err != nil {
		return types.ShareProof{}, err
	}

	edsColRoots, err := eds.ColRoots()
	if err != nil {
		return types.ShareProof{}, err
	}

	// create the binary merkle inclusion proof for all the square rows to the data root
	_, allProofs := merkle.ProofsFromByteSlices(append(edsRowRoots, edsColRoots...))
	rowProofs := make([]*merkle.Proof, endRow-startRow+1)
	rowRoots := make([]tmbytes.HexBytes, endRow-startRow+1)
	for i := startRow; i <= endRow; i++ {
		rowProofs[i-startRow] = allProofs[i]
		rowRoots[i-startRow] = edsRowRoots[i]
	}

	// get the extended rows containing the shares.
	rows := make([][]shares.Share, endRow-startRow+1)
	for i := startRow; i <= endRow; i++ {
		shares, err := shares.FromBytes(eds.Row(uint(i)))
		if err != nil {
			return types.ShareProof{}, err
		}
		rows[i-startRow] = shares
	}

	var shareProofs []*tmproto.NMTProof //nolint:prealloc
	var rawShares [][]byte
	for i, row := range rows {
		// create an nmt to generate a proof.
		// we have to re-create the tree as the eds one is not accessible.
		tree := wrapper.NewErasuredNamespacedMerkleTree(uint64(squareSize), uint(i))
		for _, share := range row {
			err := tree.Push(
				share.ToBytes(),
			)
			if err != nil {
				return types.ShareProof{}, err
			}
		}

		// make sure that the generated root is the same as the eds row root.
		root, err := tree.Root()
		if err != nil {
			return types.ShareProof{}, err
		}
		if !bytes.Equal(rowRoots[i].Bytes(), root) {
			return types.ShareProof{}, errors.New("eds row root is different than tree root")
		}

		startLeafPos := startLeaf
		endLeafPos := endLeaf

		// if this is not the first row, then start with the first leaf
		if i > 0 {
			startLeafPos = 0
		}
		// if this is not the last row, then select for the rest of the row
		if i != (len(rows) - 1) {
			endLeafPos = squareSize - 1
		}

		rawShares = append(rawShares, shares.ToBytes(row[startLeafPos:endLeafPos+1])...)
		proof, err := tree.ProveRange(int(startLeafPos), int(endLeafPos+1))
		if err != nil {
			return types.ShareProof{}, err
		}

		shareProofs = append(shareProofs, &tmproto.NMTProof{
			Start:              int32(proof.Start()),
			End:                int32(proof.End()),
			PreIndex:           int32(proof.PreIndex()),
			PostIndex:          int32(proof.PostIndex()),
			OpenStart:          types.FromNmtKzgOpen(proof.OpenStart()),
			OpenEnd:            types.FromNmtKzgOpen(proof.OpenEnd()),
			OpenPreIndex:       types.FromNmtKzgOpen(proof.OpenPreIndex()),
			OpenPostIndex:      types.FromNmtKzgOpen(proof.OpenPostIndex()),
			InclusionOrAbsence: proof.InclusionOrAbsence,
		})
	}

	return types.ShareProof{
		RowProof: types.RowProof{
			RowRoots: rowRoots,
			Proofs:   rowProofs,
			StartRow: uint32(startRow),
			EndRow:   uint32(endRow),
		},
		Data:             rawShares,
		ShareProofs:      shareProofs,
		NamespaceID:      namespace.ID,
		NamespaceVersion: uint32(namespace.Version),
	}, nil
}

const TxInclusionQueryPath = "txInclusionProof"

// Querier defines the logic performed when the ABCI client using the Query
// method with the custom prove.QueryPath. The index of the transaction being
// proved must be appended to the path. The marshalled bytes of the transaction
// proof (tmproto.ShareProof) are returned.
//
// example path for proving the third transaction in that block:
// custom/txInclusionProof/3
func QueryTxInclusionProof(_ sdk.Context, path []string, req abci.RequestQuery) ([]byte, error) {
	// parse the index from the path
	if len(path) != 1 {
		return nil, fmt.Errorf("expected query path length: 1 actual: %d ", len(path))
	}
	index, err := strconv.ParseInt(path[0], 10, 64)
	if err != nil {
		return nil, err
	}

	// unmarshal the block data that is passed from the ABCI client
	pbb := new(tmproto.Block)
	err = pbb.Unmarshal(req.Data)
	if err != nil {
		return nil, fmt.Errorf("error reading block: %w", err)
	}
	data, err := types.DataFromProto(&pbb.Data)
	if err != nil {
		panic(fmt.Errorf("error from proto block: %w", err))
	}

	// create and marshal the tx inclusion proof, which we return in the form of []byte
	shareProof, err := NewTxInclusionProof(data.Txs.ToSliceOfBytes(), uint64(index), pbb.Header.Version.App)
	if err != nil {
		return nil, err
	}
	pShareProof := shareProof.ToProto()
	rawShareProof, err := pShareProof.Marshal()
	if err != nil {
		return nil, err
	}

	return rawShareProof, nil
}

const ShareInclusionQueryPath = "shareInclusionProof"

// QueryShareInclusionProof defines the logic performed when querying for the
// inclusion proofs of a set of shares to the data root. The share range should
// be appended to the path. Example path for proving the set of shares [3, 5]:
// custom/shareInclusionProof/3/5
func QueryShareInclusionProof(_ sdk.Context, path []string, req abci.RequestQuery) ([]byte, error) {
	// parse the share range from the path
	if len(path) != 2 {
		return nil, fmt.Errorf("expected query path length: 2 actual: %d ", len(path))
	}
	beginShare, err := strconv.ParseInt(path[0], 10, 64)
	if err != nil {
		return nil, err
	}
	endShare, err := strconv.ParseInt(path[1], 10, 64)
	if err != nil {
		return nil, err
	}

	// unmarshal the block data that is passed from the ABCI client
	pbb := new(tmproto.Block)
	err = pbb.Unmarshal(req.Data)
	if err != nil {
		return nil, fmt.Errorf("error reading block: %w", err)
	}

	// construct the data square from the block data. As we don't have
	// access to the application's state machine we use the upper bound
	// square size instead of the square size dictated from governance
	dataSquare, err := square.Construct(pbb.Data.Txs, pbb.Header.Version.App, appconsts.SquareSizeUpperBound(pbb.Header.Version.App))
	if err != nil {
		return nil, err
	}

	nID, err := ParseNamespace(dataSquare, int(beginShare), int(endShare))
	if err != nil {
		return nil, err
	}

	// create and marshal the share inclusion proof, which we return in the form of []byte
	shareProof, err := NewShareInclusionProof(
		dataSquare,
		nID,
		shares.NewRange(int(beginShare), int(endShare)),
	)
	if err != nil {
		return nil, err
	}
	pShareProof := shareProof.ToProto()
	rawShareProof, err := pShareProof.Marshal()
	if err != nil {
		return nil, err
	}

	return rawShareProof, nil
}

// ParseNamespace validates the share range, checks if it only contains one namespace and returns
// that namespace ID.
func ParseNamespace(rawShares []shares.Share, startShare, endShare int) (appns.Namespace, error) {
	if startShare < 0 {
		return appns.Namespace{}, fmt.Errorf("start share %d should be positive", startShare)
	}

	if endShare < 0 {
		return appns.Namespace{}, fmt.Errorf("end share %d should be positive", endShare)
	}

	if endShare < startShare {
		return appns.Namespace{}, fmt.Errorf("end share %d cannot be lower than starting share %d", endShare, startShare)
	}

	if endShare >= len(rawShares) {
		return appns.Namespace{}, fmt.Errorf("end share %d is higher than block shares %d", endShare, len(rawShares))
	}

	startShareNs, err := rawShares[startShare].Namespace()
	if err != nil {
		return appns.Namespace{}, err
	}

	for i, share := range rawShares[startShare:endShare] {
		ns, err := share.Namespace()
		if err != nil {
			return appns.Namespace{}, err
		}
		if !bytes.Equal(startShareNs.Bytes(), ns.Bytes()) {
			return appns.Namespace{}, fmt.Errorf("shares range contain different namespaces at index %d: %v and %v ", i, startShareNs, ns)
		}
	}
	return startShareNs, nil
}

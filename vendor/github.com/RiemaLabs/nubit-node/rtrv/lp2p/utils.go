package ipld

import (
	"github.com/ipfs/go-cid"

	share "github.com/RiemaLabs/nubit-node/da"
)

// FilterRootByNamespace returns the row roots from the given share.Root that contain the namespace.
func FilterRootByNamespace(root *share.Root, namespace share.Namespace) []cid.Cid {
	rowRootCIDs := make([]cid.Cid, 0, len(root.RowRoots))
	for _, row := range root.RowRoots {
		if !namespace.IsOutsideRange(row, row) {
			rowRootCIDs = append(rowRootCIDs, MustCidFromKzg(row[2*share.NamespaceSize:]))
		}
	}
	return rowRootCIDs
}

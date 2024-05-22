# rsmt2d
Go implementation of two dimensional Reed-Solomon sqaure, the underlying data structure for data availability, with KZG-based vector commitment and Merkle tree.

# Design
We arrange the DA datas of each block in the form of data squares. For instance, for a block containing $N$ data shares, $\llcorner\sqrt{N}\lrcorner$ is set to be the square width and $\lceil N/\llcorner\sqrt{N}\lrcorner\rceil$ is set to be the square height. Our data square is designed to provide such a data root while allowing for efficient and secure light clients. The general concept of encoding the data sqaure is to apply vector commitments (see below) for each data row (column) and aggregate the row (column) commitments via a Merkle tree.

## Data Square

To facilitate descriptions, we notate a data square as $A=[R_1, R_2, \ldots, R_h]^T$. Each $R_i$ represents a row. 

To generate the data root, we firstly rearrange the datas as a sqaure (as above). Noticeably, the datas are sorted according to namespaces before the rearrangement.

Secondly, assuming $H(\cdot)$ is a cryptographic hash function (`SHA-256` for the moment), for each row $R_i$, generate  $\mathsf{commit}\left(\mathsf{gp}, (H(R_{i,1}),H(R_{i,2}),\ldots,H(R_{i,w}))\right)\to \mathsf{com}_{i}$. Then, also generate $\mathsf{com}’_i$ for each row $i\in[w]$.

The final data root is the Merkle root of all $\mathsf{com}_i$’s and $\mathsf{com}’_i$’s.

## Storing the Data Square

We store the data square via CAR files (to facilitate IPLD transmission). For each data sqaure, KZG-based vector commitments are generated for each row, denoted as $C_i~(i\in[h])$.

The data availability header of the block contains a map from each $i\in[h]$ to the commitment $C_i$.  The `Cid` (i.e. the identifier of a data package either in the IPLD network or local storage) of the row datas is $H(C_i)$. This Cid maps to the hashes of the shares of the row, and each hash of also the `Cid` of the share data. For example, for a row $R_i$, the row `Cid` maps to a CAR including $H(R_{i,1}),H(R_{i,2}),\ldots,H(R_{i,w})$. Each $H(R_{i,j})$ itself, is also a valid 32-byte `Cid` address and maps to the data $R_{i,j}$. Noticeably, $R_{i,j}$ consists of a share and the namespace of the share.

## License

Dual-licensed: [MIT](./LICENSE-MIT), [Apache Software License v2](./LICENSE-APACHE), by way of the
[Permissive License Stack](https://protocol.ai/blog/announcing-the-permissive-license-stack/).
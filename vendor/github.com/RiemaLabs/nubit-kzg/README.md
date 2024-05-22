# Nubit-KZG
Go implementation of data encoding via the Kate-Zaverucha-Goldberg (KZG) polynomial commitment scheme.

# Design
## KZG commitments

The Kate-Zaverucha-Goldberg (KZG) polynomial commitment scheme is possibly the most well-known polynomial commitment, however, its was designed to commit a polynomial instead of a vector of data.

However, after being equipped with a polynomial interpolation, KZG has been widely adopted in the cryptocurrency world to function as vector polynomials. Actually, there are alternative schemes specialized for vector commitments, however, most of them are less efficient, stronger assumption on trusted setups, or involving a larger proof size. For the moment, “KZG + polynomiala interpolation” is still be best option for our vector commitment.

Formally, a (KZG-based) vector commitment scheme consists of the following subroutines.

(1) Setup:  $\mathsf{setup}(1^\kappa)\to \mathsf{gp}$.
The setup algorithm samples a random $\tau$ and returns $\mathsf{gp}=(G,\tau\cdot G,\tau^2\cdot G,\ldots,\tau^d\cdot G)$.
This seemingly trustful setup would be realized in a trustless manner by our designs.

(2) Commitment to a vector: $\mathsf{commit}(\mathsf{gp},\mathbf{v}=(v_1,v_2,\ldots,v_\ell)) \to \mathsf{com}_{\mathbf{v}}.$ 

The commitment algorithm first interpolates a polynomial as $f(X)={\sum_{i=1}}^\ell v_i\cdot \mathsf{L}_i(X),$
where $\mathsf{L}_i(X)$ is the Lagrange base of the index $i$.

Then, it returns $\mathsf{com}_{\mathbf{v}}:=f(\tau)\cdot G$. Notably, although $\tau$ is not contained in $\mathsf{gp}$, this value can be calculated since $\tau^i\cdot G$ is contained for each $i\in[\ell]$.

(3) Opening: $\mathsf{open}(\mathsf{gp},\mathbf{v},i)\to (v_i,\pi_i)$. To open a value of index $i$, the opening algorithm returns $v_i$ and $\pi_i=\frac{f(\tau)-v_i}{\tau-i}\cdot G$,
where $f$ is the interpolation of $\mathbf{v}$.

(4) Verification: $\mathsf{verif}(\mathsf{gp},\mathsf{com}_{\mathbf{v}},i,v_i,\pi_i)\to 0/1$.
The verification algorithm succeeds only if $e\left(\left(\tau-i\right)\cdot G, \pi_i\right) = e\left( (f(\tau)-v_i)\cdot G , G \right)$.

## Opens for Commitments

```go
type KzgOpen struct {
	index int
	value []byte
	proof KzgProof
}
```

To open a specified slot (holding the row commitment) for $R_i$, a proof $\pi$ is presented along with the index $i$ and the value $v_i$. The verification can be made by triggering $\mathsf{verif}(\mathsf{gp},\mathsf{com}_{\mathbf{v}},i,H(v_i),\pi_i)$.

## Finding Namespaced Datas in the Square

Recall that the shares are sorted by namespaces in the lexicographic order. This means a binary search would suffice for finding all datas of one specific namespace. However, in the trustless environment, it is not trivial to prove to users that a returned list of shares is the entire set of shares of the namespace. Hence, delicated proofs (see below) are designed to meet this goal.

## Proofs for Namespaced Datas

Most of the time, users from the application end would only be interested in the datas of one specified namespace.

Namespaced range proofs are designed to meet the purpose and provide a proof of (1) the inclusion of datas of the namespace; or (2) the absence of the namespace.

```go
type NamespaceRangeProof struct {
	// [start,end]
	start int 
	end int

	// Ideally preIndex == start-1 and postIndex == end+1, however,
	// there might be no value of concerned namespace.
	// (preIndex, postIndex)
	preIndex  int 
	postIndex int

	openStart     KzgOpen
	openEnd       KzgOpen
	openPreIndex  KzgOpen
	openPostIndex KzgOpen

	// TRUE for Inclusion; FALSE for Absence
	InclusionOrAbsence bool
}
```

The `NamespaceRangeProof` data structure represents an inclusion proof if `InclusionOrAbsence=True` and an absence proof in the other case.

- Inclusion proofs. For a namespace $ns$, if it appears in the concerned data row,

`[start,end]` indicates the range of its appearance, `preIndex=start-1`, and `postIndex=end+1`.

`openStart`, `openEnd`, `openPreIndex`, and `openPostIndex` are the KZG openings for `start`, `end`, `preIndex`, and `postIndex`, respectively.

Let $v_s, v_t, v_{pre}, v_{post}$ be the opened values above. Ideally, $v_{pre} < v_s=v_t=ns < v_{post}$.

Howevere, there are corner cases to handle properly, say, the case of `start=0` or `end=w-1`.

- Absence Proofs.

Traditionally, proving non-existence, or proving a universal quantifier involves enumerating all datas. However, we have sorted the datas according to the namespaces.

Noticeably, such a sorting is conducted by block proposer and is verified by the validators. Its correctness is guaranteed by the safety of the consensus and its integrity is guaranteed by our data availability layer.

To prove the absence of a specific namespace, `start` and `end` are meaningless. Instead, `preIndex` should equal to `postIndex-1` ,`openPreIndex` should contain a namespace smaller than `ns` while `openPostIndex` should contain a namespace greater than `ns`.

There are also corner cases to consider, say, if `preIndex=0` or `postIndex=w-1`, the other index is meaningless and the proof only involves the opening of one slot.

## License

Dual-licensed: [MIT](./LICENSE-MIT), [Apache Software License v2](./LICENSE-APACHE), by way of the
[Permissive License Stack](https://protocol.ai/blog/announcing-the-permissive-license-stack/).
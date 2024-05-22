package rsmt2d

import (
	"crypto/sha256"
	"fmt"
	"hash"

	gokzg4844 "github.com/RiemaLabs/go-kzg-4844"
	"github.com/consensys/gnark-crypto/ecc/bls12-381/fr"
)

const (
	AllowedMaxSize = 4096
)

var (
	ErrMaxSizeExceeded = fmt.Errorf("max size exceeded")
)

var _ Tree = &Kzg{}

type Kzg struct {
	ctx     *gokzg4844.Context
	hasher  hash.Hash
	maxSize uint32
	leaves  [][]byte
	root    []byte
}

func NewKzg(h hash.Hash) *Kzg {
	ctx, _ := gokzg4844.NewContext4096Secure()
	return &Kzg{
		ctx:     ctx,
		maxSize: AllowedMaxSize,
		hasher:  h,
		leaves:  make([][]byte, 0, AllowedMaxSize),
		root:    nil,
	}
}

func (k *Kzg) Root() ([]byte, error) {
	if k.root != nil {
		return k.root, nil
	}
	hasher := k.hasher
	blob := gokzg4844.Blob{}
	for i, l := range k.leaves {
		hasher.Reset()
		hasher.Write(l)
		h := hasher.Sum(nil)

		var sc fr.Element
		sc.SetBytes(h)
		res := sc.Bytes()

		copy(blob[i*gokzg4844.SerializedScalarSize:(i+1)*gokzg4844.SerializedScalarSize], res[:])
	}

	c, err := k.ctx.BlobToKZGCommitment(&blob, 0)
	if err != nil {
		return nil, err
	}
	k.root = c[:]
	return k.root, nil
}

func (k *Kzg) Push(data []byte) error {
	if uint32(len(k.leaves)) >= k.maxSize {
		return ErrMaxSizeExceeded
	}
	k.leaves = append(k.leaves, data)
	return nil
}

func NewKzgTree(_ Axis, _ uint) Tree {
	return NewKzg(sha256.New())
}

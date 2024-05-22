package gokzg4844

import (
	"github.com/consensys/gnark-crypto/ecc/bls12-381/fr"
)

func (c *Context) DomainByIndex(index int) (*fr.Element, error) {
	if index > int(c.domain.Cardinality) {
		return nil, ErrIndexOutOfRange
	}

	return &c.domain.Roots[index], nil
}

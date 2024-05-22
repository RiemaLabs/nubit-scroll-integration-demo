package shares

import (
	"encoding/binary"
	"fmt"

	"github.com/RiemaLabs/nubit-validator/utils/appconsts"
	"golang.org/x/exp/constraints"
)

// RoundUpPowerOfTwo returns the next power of two greater than or equal to input.
func RoundUpPowerOfTwo[I constraints.Integer](input I) I {
	var result I = 1
	for result < input {
		result = result << 1
	}
	return result
}

// RoundDownPowerOfTwo returns the next power of two less than or equal to input.
func RoundDownPowerOfTwo[I constraints.Integer](input I) (I, error) {
	if input <= 0 {
		return 0, fmt.Errorf("input %v must be positive", input)
	}
	roundedUp := RoundUpPowerOfTwo(input)
	if roundedUp == input {
		return roundedUp, nil
	}
	return roundedUp / 2, nil
}

// RoundUpPowerOfTwo returns the next power of two that is strictly greater than input.
func RoundUpPowerOfTwoStrict[I constraints.Integer](input I) I {
	result := RoundUpPowerOfTwo(input)

	// round the result up to the next power of two if is equal to the input
	if result == input {
		return result * 2
	}
	return result
}

// IsPowerOfTwo returns true if input is a power of two.
func IsPowerOfTwo[I constraints.Integer](input I) bool {
	return input&(input-1) == 0 && input != 0
}

// Range is an end exclusive set of share indexes.
type Range struct {
	// Start is the index of the first share occupied by this range.
	Start int
	// End is the next index after the last share occupied by this range.
	End int
}

func NewRange(start, end int) Range {
	return Range{Start: start, End: end}
}

func EmptyRange() Range {
	return Range{Start: 0, End: 0}
}

func (r Range) IsEmpty() bool {
	return r.Start == 0 && r.End == 0
}

func (r *Range) Add(value int) {
	r.Start += value
	r.End += value
}

// NewReservedBytes returns a byte slice of length
// appconsts.CompactShareReservedBytes that contains the byteIndex of the first
// unit that starts in a compact share.
func NewReservedBytes(byteIndex uint32) ([]byte, error) {
	if byteIndex >= appconsts.ShareSize {
		return []byte{}, fmt.Errorf("byte index %d must be less than share size %d", byteIndex, appconsts.ShareSize)
	}
	reservedBytes := make([]byte, appconsts.CompactShareReservedBytes)
	binary.BigEndian.PutUint32(reservedBytes, byteIndex)
	return reservedBytes, nil
}

// ParseReservedBytes parses a byte slice of length
// appconsts.CompactShareReservedBytes into a byteIndex.
func ParseReservedBytes(reservedBytes []byte) (uint32, error) {
	if len(reservedBytes) != appconsts.CompactShareReservedBytes {
		return 0, fmt.Errorf("reserved bytes must be of length %d", appconsts.CompactShareReservedBytes)
	}
	byteIndex := binary.BigEndian.Uint32(reservedBytes)
	if appconsts.ShareSize <= byteIndex {
		return 0, fmt.Errorf("byteIndex must be less than share size %d", appconsts.ShareSize)
	}
	return byteIndex, nil
}

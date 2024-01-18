package types

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPreimageOracleData_LeafCount(t *testing.T) {
	tests := []struct {
		name     string
		data     *PreimageOracleData
		expected uint32
	}{
		{
			name:     "EmptyData",
			data:     NewPreimageOracleData([]byte{}, []byte{}, 0),
			expected: 0,
		},
		{
			name:     "SingleBlockData",
			data:     NewPreimageOracleData([]byte{}, make([]byte, 136), 0),
			expected: 1,
		},
		{
			name:     "MultiBlockData",
			data:     NewPreimageOracleData([]byte{}, make([]byte, 136*2), 0),
			expected: 2,
		},
		{
			name:     "MultiBlockDataWithPartialBlock",
			data:     NewPreimageOracleData([]byte{}, make([]byte, 136*2+1), 0),
			expected: 3,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.expected, test.data.LeafCount())
		})
	}
}

func TestPreimageOracleData_GetKeccakLeaf(t *testing.T) {
	fullBlock := make([]byte, 136)
	for i := 0; i < 136; i++ {
		fullBlock[i] = byte(i)
	}
	tests := []struct {
		name     string
		data     *PreimageOracleData
		offset   uint32
		expected []byte
	}{
		{
			name:     "SingleBlockData",
			data:     NewPreimageOracleData([]byte{}, fullBlock, 0),
			offset:   0,
			expected: fullBlock,
		},
		{
			name:     "MultiBlockData",
			data:     NewPreimageOracleData([]byte{}, append(fullBlock, fullBlock...), 0),
			offset:   1,
			expected: fullBlock,
		},
		{
			name:     "MultiBlockDataWithPartialBlock",
			data:     NewPreimageOracleData([]byte{}, append(fullBlock, byte(9)), 0),
			offset:   1,
			expected: append(make([]byte, LibKeccakBlockSizeBytes-1), byte(9)),
		},
		{
			name:     "OffsetOverflow",
			data:     NewPreimageOracleData([]byte{}, append(fullBlock, byte(9)), 0),
			offset:   2,
			expected: make([]byte, LibKeccakBlockSizeBytes),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.expected, test.data.GetKeccakLeaf(test.offset))
		})
	}
}

func TestNewPreimageOracleData(t *testing.T) {
	t.Run("LocalData", func(t *testing.T) {
		data := NewPreimageOracleData([]byte{1, 2, 3}, []byte{4, 5, 6}, 7)
		require.True(t, data.IsLocal)
		require.Equal(t, []byte{1, 2, 3}, data.OracleKey)
		require.Equal(t, []byte{4, 5, 6}, data.OracleData)
		require.Equal(t, uint32(7), data.OracleOffset)
	})

	t.Run("GlobalData", func(t *testing.T) {
		data := NewPreimageOracleData([]byte{0, 2, 3}, []byte{4, 5, 6}, 7)
		require.False(t, data.IsLocal)
		require.Equal(t, []byte{0, 2, 3}, data.OracleKey)
		require.Equal(t, []byte{4, 5, 6}, data.OracleData)
		require.Equal(t, uint32(7), data.OracleOffset)
	})
}

func TestIsRootPosition(t *testing.T) {
	tests := []struct {
		name     string
		position Position
		expected bool
	}{
		{
			name:     "ZeroRoot",
			position: NewPositionFromGIndex(big.NewInt(0)),
			expected: true,
		},
		{
			name:     "ValidRoot",
			position: NewPositionFromGIndex(big.NewInt(1)),
			expected: true,
		},
		{
			name:     "NotRoot",
			position: NewPositionFromGIndex(big.NewInt(2)),
			expected: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.expected, test.position.IsRootPosition())
		})
	}
}

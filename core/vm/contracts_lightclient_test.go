package vm

import (
	"encoding/binary"
	"encoding/hex"
	"testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetMinerAndMerkleProofValidate(t *testing.T) {
	data, _ := hex.DecodeString("00000030b1af0585bd9ecb320ef85a42debb3ef2d0ccfb535b2075e28d58dc3e8959955dd38002be969899cf593e3b8a69d8b23c2d8d9e8420646829f4f818dcda288c8f9e68ba62ffff7f2001000000020000000001010000000000000000000000000000000000000000000000000000000000000000ffffffff050222010101ffffffff0200f90295000000001976a914210fcf4212bdb75c8692863ddb8cf481d613532c88ac0000000000000000266a24aa21a9ede2f61c3f71d1defd3fa999dfa36953755c690689799962b48bebd836974e8cf9012000000000000000000000000000000000000000000000000000000000000000000000000000")

	totalLengthPrefix := make([]byte, 32)
	binary.BigEndian.PutUint64(totalLengthPrefix[0:8], 0)
	binary.BigEndian.PutUint64(totalLengthPrefix[8:16], 0)
	binary.BigEndian.PutUint64(totalLengthPrefix[16:24], 0)
	binary.BigEndian.PutUint64(totalLengthPrefix[24:], uint64(len(data)))
	input := append(totalLengthPrefix, data...)

	c := btcValidate{}
	output, err := c.Run(input)
	require.NoError(t, err)

	typ := binary.BigEndian.Uint32(output[60:])
	assert.Equal(t, typ, uint32(2), "type error")
	addr := hex.EncodeToString((output[:20]))
	assert.Equal(t, addr, "210fcf4212bdb75c8692863ddb8cf481d613532c", "address error")
}

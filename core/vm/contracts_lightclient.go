package vm

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/coredao-org/btcpowermirror/lightmirror"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/params"
)

const (
	uint64TypeLength                      uint64 = 8
	precompileContractInputMetaDataLength uint64 = 32
)

// btcValidate implemented as a precompiled contract.
type btcValidate struct{}

func (c *btcValidate) RequiredGas(input []byte) uint64 {
	return params.BitcoinHeaderValidateGas + uint64(len(input)/32)*params.IAVLMerkleProofValidateGas
}

func (c *btcValidate) Run(input []byte) (result []byte, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("internal error: %v\n", r)
		}
	}()

	if uint64(len(input)) <= precompileContractInputMetaDataLength {
		return nil, fmt.Errorf("invalid input")
	}

	payloadLength := binary.BigEndian.Uint64(input[precompileContractInputMetaDataLength-uint64TypeLength : precompileContractInputMetaDataLength])
	if uint64(len(input)) != payloadLength+precompileContractInputMetaDataLength {
		return nil, fmt.Errorf("invalid input: input size should be %d, actual size is %d", payloadLength+precompileContractInputMetaDataLength, len(input))
	}

	rbuf := bytes.NewReader(input[precompileContractInputMetaDataLength:])
	var mirror lightmirror.BtcLightMirror
	err = mirror.Deserialize(rbuf)
	if err != nil {
		err = fmt.Errorf("deserialize btcLightMirror failed %s", err.Error())
		return nil, err
	}

	// Verify MerkleRoot & coinbaseTx
	err = mirror.CheckMerkle()
	if err != nil {
		err = fmt.Errorf("verify btcLightMirror failed %s", err.Error())
		return nil, err
	}

	coinbaseAddr, addrType := mirror.GetCoinbaseAddress()
	// result
	// | coinbaseAddr        |
	// | 20 bytes + 12 bytes |
	addrTypeBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(addrTypeBytes, (uint32)(addrType))
	addrTypeBytes = common.LeftPadBytes(addrTypeBytes[:], 32)
	return append(common.RightPadBytes(coinbaseAddr[:], 32), addrTypeBytes...), nil
}

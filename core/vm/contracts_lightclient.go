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

// btcValidateV2 implemented as a precompiled contract.
type btcValidateV2 struct{}

func (c *btcValidateV2) RequiredGas(input []byte) uint64 {
	return params.BitcoinHeaderValidateBaseGas + uint64(len(input)/32)*params.BitcoinHeaderValidatePerWordGas
}

func (c *btcValidateV2) Run(input []byte) (result []byte, err error) {
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
	var mirror lightmirror.BtcLightMirrorV2
	err = mirror.Deserialize(rbuf)
	if err != nil {
		err = fmt.Errorf("deserialize BtcLightMirrorV2 failed %s", err.Error())
		return nil, err
	}

	// Verify MerkleRoot & coinbaseTx
	err = mirror.CheckMerkle()
	if err != nil {
		err = fmt.Errorf("verify BtcLightMirrorV2 failed %s", err.Error())
		return nil, err
	}

	candidateAddr, rewardAddr, blockHash := mirror.ParsePowerParams()

	res := make([]byte, 0, 96)
	res = append(res, common.LeftPadBytes(candidateAddr.Bytes(), 32)...)
	res = append(res, common.LeftPadBytes(rewardAddr.Bytes(), 32)...)
	res = append(res, blockHash[:]...)
	return res, nil
}

package satoshi

import (
	"context"
	"fmt"
	"math"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/systemcontracts"
	"github.com/ethereum/go-ethereum/internal/ethapi"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/rpc"
)

// GetValidators retrieves validator operator addresses from the StakeHubContract.
//
// The on-chain method used here is `getCandidates()` which returns only operator
// addresses.
func (p *Satoshi) GetValidators() ([]common.Address, error) {
	log.Debug("Getting validators from latest block")

	// Create the call data for getCandidates
	data, err := p.candidateHubABI.Pack("getCandidates")
	if err != nil {
		log.Error("Failed to pack stakehub getCandidates", "error", err)
		return nil, fmt.Errorf("failed to pack getCandidates: %v", err)
	}

	// Make the call
	blockNr := rpc.BlockNumberOrHashWithNumber(rpc.LatestBlockNumber)
	msgData := (hexutil.Bytes)(data)
	toAddress := common.HexToAddress(systemcontracts.StakeHubContract)
	gas := (hexutil.Uint64)(uint64(math.MaxUint64 / 2))

	log.Debug("Calling getCandidates from latest block", "to", toAddress)
	result, err := p.ethAPI.Call(context.Background(), ethapi.TransactionArgs{
		Gas:  &gas,
		To:   &toAddress,
		Data: &msgData,
	}, &blockNr, nil, nil)
	if err != nil {
		log.Error("Failed to call stakehub getCandidates", "error", err)
		return nil, fmt.Errorf("failed to call stakehub getCandidates: %v", err)
	}

	// Unpack the result
	var operatorAddrs []common.Address
	if err := p.candidateHubABI.UnpackIntoInterface(&operatorAddrs, "getCandidates", result); err != nil {
		log.Error("Failed to unpack stakehub getCandidates result", "error", err)
		return nil, fmt.Errorf("failed to unpack getCandidates result: %v", err)
	}

	log.Debug("Successfully retrieved stakehub candidates", "operators", len(operatorAddrs))
	return operatorAddrs, nil
}

// getNodeIDsForValidators retrieves node IDs for the given validators
// It returns a map of consensus addresses to their node IDs
func (p *Satoshi) getNodeIDsForValidators(validatorsToQuery []common.Address) (map[common.Address][]enode.ID, error) {
	log.Debug("Listing node IDs for validators from latest block", "validators", len(validatorsToQuery))

	// Create the call data for getNodeIDs
	data, err := p.candidateHubABI.Pack("getNodeIDs", validatorsToQuery)
	if err != nil {
		log.Error("Failed to pack getNodeIDs", "error", err)
		return nil, fmt.Errorf("failed to pack getNodeIDs: %v", err)
	}

	// Make the call
	blockNr := rpc.BlockNumberOrHashWithNumber(rpc.LatestBlockNumber)
	msgData := (hexutil.Bytes)(data)
	toAddress := common.HexToAddress(systemcontracts.StakeHubContract)
	gas := (hexutil.Uint64)(uint64(math.MaxUint64 / 2))

	log.Debug("Calling getNodeIDs from latest block", "to", toAddress)
	result, err := p.ethAPI.Call(context.Background(), ethapi.TransactionArgs{
		Gas:  &gas,
		To:   &toAddress,
		Data: &msgData,
	}, &blockNr, nil, nil)
	if err != nil {
		log.Error("Failed to call getNodeIDs", "error", err)
		return nil, fmt.Errorf("failed to call getNodeIDs: %v", err)
	}

	// Unpack the result
	var consensusAddresses []common.Address
	var nodeIDsList [][]enode.ID
	if err := p.candidateHubABI.UnpackIntoInterface(&[]interface{}{&consensusAddresses, &nodeIDsList}, "getNodeIDs", result); err != nil {
		log.Error("Failed to unpack getNodeIDs result", "error", err)
		return nil, fmt.Errorf("failed to unpack getNodeIDs result: %v", err)
	}

	// Create a map of addresses to node IDs
	addressToNodeIDs := make(map[common.Address][]enode.ID)
	for i, addr := range consensusAddresses {
		if i < len(nodeIDsList) {
			addressToNodeIDs[addr] = nodeIDsList[i]
		}
	}

	log.Debug("Successfully retrieved node IDs", "addresses", len(addressToNodeIDs))
	return addressToNodeIDs, nil
}

// GetNodeIDs returns a flattened array of all node IDs for current validators
func (p *Satoshi) GetNodeIDs() ([]enode.ID, error) {
	// Call GetValidators with latest block number
	operatorAddrs, err := p.GetValidators()
	if err != nil {
		log.Error("Failed to get validators", "error", err)
		return nil, fmt.Errorf("failed to get validators: %v", err)
	}
	log.Debug("Retrieved validators", "count", len(operatorAddrs))

	// Get node IDs for validators
	nodeIDs, err := p.getNodeIDsForValidators(operatorAddrs)
	if err != nil {
		log.Error("Failed to get node IDs", "error", err)
		return nil, fmt.Errorf("failed to get node IDs: %v", err)
	}
	log.Debug("Retrieved node IDs map", "addresses", len(nodeIDs))

	// Flatten the array of arrays into a single array
	flatNodeIDs := make([]enode.ID, 0)
	for addr, nodeIDArray := range nodeIDs {
		flatNodeIDs = append(flatNodeIDs, nodeIDArray...)
		log.Debug("Processing node IDs", "address", addr, "count", len(nodeIDArray))
	}

	log.Debug("Successfully flattened node IDs", "total", len(flatNodeIDs))
	return flatNodeIDs, nil
}

// GetNodeIDsMap returns a map of consensus addresses to their node IDs for all current validators
func (p *Satoshi) GetNodeIDsMap() (map[common.Address][]enode.ID, error) {
	// Call GetValidators with latest block number
	operatorAddrs, err := p.GetValidators()
	if err != nil {
		log.Error("Failed to get validators", "error", err)
		return nil, fmt.Errorf("failed to get validators: %v", err)
	}
	log.Debug("Retrieved validators", "count", len(operatorAddrs))

	// Get node IDs for validators
	nodeIDsMap, err := p.getNodeIDsForValidators(operatorAddrs)
	if err != nil {
		log.Error("Failed to get node IDs", "error", err)
		return nil, fmt.Errorf("failed to get node IDs: %v", err)
	}
	log.Debug("Retrieved node IDs map", "addresses", len(nodeIDsMap))

	return nodeIDsMap, nil
}

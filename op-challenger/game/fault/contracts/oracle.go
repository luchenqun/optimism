package contracts

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum-optimism/optimism/op-bindings/bindings"
	"github.com/ethereum-optimism/optimism/op-challenger/game/fault/types"
	gameTypes "github.com/ethereum-optimism/optimism/op-challenger/game/types"
	"github.com/ethereum-optimism/optimism/op-service/sources/batching"
	"github.com/ethereum-optimism/optimism/op-service/txmgr"
	"github.com/ethereum/go-ethereum/common"
)

const (
	methodLoadKeccak256PreimagePart = "loadKeccak256PreimagePart"
	methodProposalCount             = "proposalCount"
	methodProposals                 = "proposals"
)

// PreimageOracleContract is a binding that works with contracts implementing the IPreimageOracle interface
type PreimageOracleContract struct {
	addr        common.Address
	multiCaller *batching.MultiCaller
	contract    *batching.BoundContract
}

func NewPreimageOracleContract(addr common.Address, caller *batching.MultiCaller) (*PreimageOracleContract, error) {
	mipsAbi, err := bindings.PreimageOracleMetaData.GetAbi()
	if err != nil {
		return nil, fmt.Errorf("failed to load preimage oracle ABI: %w", err)
	}

	return &PreimageOracleContract{
		addr:        addr,
		multiCaller: caller,
		contract:    batching.NewBoundContract(mipsAbi, addr),
	}, nil
}

func (c *PreimageOracleContract) Addr() common.Address {
	return c.addr
}

func (c *PreimageOracleContract) AddGlobalDataTx(data *types.PreimageOracleData) (txmgr.TxCandidate, error) {
	call := c.contract.Call(methodLoadKeccak256PreimagePart, new(big.Int).SetUint64(uint64(data.OracleOffset)), data.GetPreimageWithoutSize())
	return call.ToTxCandidate()
}

func (c *PreimageOracleContract) GetActivePreimages(ctx context.Context, blockHash common.Hash) ([]gameTypes.LargePreimageMetaData, error) {
	results, err := batching.ReadArray(ctx, c.multiCaller, batching.BlockByHash(blockHash), c.contract.Call(methodProposalCount), func(i *big.Int) *batching.ContractCall {
		return c.contract.Call(methodProposals, i)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to load claims: %w", err)
	}

	var proposals []gameTypes.LargePreimageMetaData
	for idx, result := range results {
		proposals = append(proposals, c.decodeProposal(result, idx))
	}
	return proposals, nil
}

func (c *PreimageOracleContract) decodeProposal(result *batching.CallResult, idx int) gameTypes.LargePreimageMetaData {
	return gameTypes.LargePreimageMetaData{
		Claimant: result.GetAddress(0),
		UUID:     result.GetBigInt(1),
	}
}

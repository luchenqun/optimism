package rpc

import (
	"context"

	"github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/ethereum-optimism/optimism/op-service/sources"
	"github.com/ethereum/go-ethereum/log"
)

var NodeRPCNamespace = "optimism"

// NodeProxyBackend implements a node rpc proxy with a leadership check before each call.
type NodeProxyBackend struct {
	log    log.Logger
	con    conductor
	client *sources.RollupClient
}

var _ NodeProxyAPI = (*NodeProxyBackend)(nil)

func NewNodeProxyBackend(log log.Logger, con conductor, client *sources.RollupClient) *NodeProxyBackend {
	return &NodeProxyBackend{
		log:    log,
		con:    con,
		client: client,
	}
}

func (api *NodeProxyBackend) SyncStatus(ctx context.Context) (*eth.SyncStatus, error) {
	if !api.con.Leader(ctx) {
		return nil, ErrNotLeader
	}
	return api.client.SyncStatus(ctx)
}

func (api *NodeProxyBackend) OutputAtBlock(ctx context.Context, blockNum uint64) (*eth.OutputResponse, error) {
	if !api.con.Leader(ctx) {
		return nil, ErrNotLeader
	}
	return api.client.OutputAtBlock(ctx, blockNum)
}

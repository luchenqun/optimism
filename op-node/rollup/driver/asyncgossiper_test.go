package driver

import (
	"context"
	"testing"
	"time"

	"github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/stretchr/testify/require"
)

type mockNetwork struct {
	reqs []*eth.ExecutionPayload
}

func (m *mockNetwork) PublishL2Payload(ctx context.Context, payload *eth.ExecutionPayload) error {
	m.reqs = append(m.reqs, payload)
	return nil
}

// TestPregossiper tests the pregossiper component
// because the component is small and simple, it is tested as a whole
// this test starts, runs, clears and stops the pregossiper
// because the pregossiper is run in an async component, it is tested with eventually
func TestPregossiper(t *testing.T) {
	m := &mockNetwork{}
	// Create a new instance of pregossiper
	p := NewPreGossiper(m)
	ctx, cancel := context.WithCancel(context.Background())

	// Start the pregossiper
	p.Start(ctx)

	// Test that the pregossiper is running within a short duration
	require.Eventually(t, func() bool {
		return p.running.Load()
	}, 100*time.Millisecond, 10*time.Millisecond)

	// send a payload
	payload := &eth.ExecutionPayload{}
	p.Gossip(payload)
	require.Eventually(t, func() bool {
		// Test that the gossiper has content at all
		return p.HasPayload() &&
			// Test that the gossiper has the correct payload
			p.Get() == payload &&
			// Test that the payload has been sent to the (mock) network
			m.reqs[0] == payload
	}, 100*time.Millisecond, 10*time.Millisecond)

	p.Clear()
	require.Eventually(t, func() bool {
		// Test that the gossiper has no payload
		return !p.HasPayload() &&
			p.Get() == nil
	}, 100*time.Millisecond, 10*time.Millisecond)

	// Stop the pregossiper
	cancel()
	// Test that the pregossiper stops within a short duration
	require.Eventually(t, func() bool {
		return !p.running.Load()
	}, 100*time.Millisecond, 10*time.Millisecond)
}

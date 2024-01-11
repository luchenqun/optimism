package async

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/log"
	"github.com/stretchr/testify/require"
)

type mockNetwork struct {
	reqs []*eth.ExecutionPayload
}

func (m *mockNetwork) PublishL2Payload(ctx context.Context, payload *eth.ExecutionPayload) error {
	m.reqs = append(m.reqs, payload)
	return nil
}

type mockMetrics struct{}

func (m *mockMetrics) RecordPublishingError() {}

// TestAsyncGossiper tests the AsyncGossiper component
// because the component is small and simple, it is tested as a whole
// this test starts, runs, clears and stops the AsyncGossiper
// because the AsyncGossiper is run in an async component, it is tested with eventually
func TestAsyncGossiper(t *testing.T) {
	m := &mockNetwork{}
	// Create a new instance of AsyncGossiper
	p := NewAsyncGossiper(context.Background(), m, log.New(), &mockMetrics{})

	// Start the AsyncGossiper
	p.Start()

	// Test that the AsyncGossiper is running within a short duration
	require.Eventually(t, func() bool {
		return p.running.Load()
	}, time.Second, 10*time.Millisecond)

	// send a payload
	payload := &eth.ExecutionPayload{
		BlockNumber: hexutil.Uint64(1),
	}
	p.Gossip(payload)
	require.Eventually(t, func() bool {
		// Test that the gossiper has content at all
		return p.Get() == payload &&
			// Test that the payload has been sent to the (mock) network
			m.reqs[0] == payload
	}, time.Second, 10*time.Millisecond)

	p.Clear()
	require.Eventually(t, func() bool {
		// Test that the gossiper has no payload
		return p.Get() == nil
	}, time.Second, 10*time.Millisecond)

	// Stop the AsyncGossiper
	p.Stop()

	// Test that the AsyncGossiper stops within a short duration
	require.Eventually(t, func() bool {
		return !p.running.Load()
	}, time.Second, 10*time.Millisecond)
}

// TestAsyncGossiperLoop confirms that when called repeatedly, the AsyncGossiper holds the latest payload
// and sends all payloads to the network
func TestAsyncGossiperLoop(t *testing.T) {
	m := &mockNetwork{}
	// Create a new instance of AsyncGossiper
	p := NewAsyncGossiper(context.Background(), m, log.New(), &mockMetrics{})

	// Start the AsyncGossiper
	p.Start()

	// Test that the AsyncGossiper is running within a short duration
	require.Eventually(t, func() bool {
		return p.running.Load()
	}, time.Second, 10*time.Millisecond)

	// send multiple payloads
	for i := 0; i < 10; i++ {
		payload := &eth.ExecutionPayload{
			BlockNumber: hexutil.Uint64(i),
		}
		p.Gossip(payload)
		require.Eventually(t, func() bool {
			// Test that the gossiper has content at all
			return p.Get() == payload &&
				// Test that the payload has been sent to the (mock) network
				m.reqs[len(m.reqs)-1] == payload
		}, time.Second, 10*time.Millisecond)
	}
	require.Equal(t, 10, len(m.reqs))
	// Stop the AsyncGossiper
	p.Stop()
	// Test that the AsyncGossiper stops within a short duration
	require.Eventually(t, func() bool {
		return !p.running.Load()
	}, time.Second, 10*time.Millisecond)
}

// failingNetwork is a mock network that always fails to publish
type failingNetwork struct{}

func (f *failingNetwork) PublishL2Payload(ctx context.Context, payload *eth.ExecutionPayload) error {
	return errors.New("failed to publish")
}

// TestAsyncGossiperFailToPublish tests that the AsyncGossiper clears the stored payload if the network fails
func TestAsyncGossiperFailToPublish(t *testing.T) {
	m := &failingNetwork{}
	// Create a new instance of AsyncGossiper
	p := NewAsyncGossiper(context.Background(), m, log.New(), &mockMetrics{})

	// Start the AsyncGossiper
	p.Start()

	// send a payload
	payload := &eth.ExecutionPayload{
		BlockNumber: hexutil.Uint64(1),
	}
	p.Gossip(payload)
	// Rather than expect the payload to become available, we should never see it, due to the publish failure
	require.Never(t, func() bool {
		return p.Get() == payload
	}, time.Second, 10*time.Millisecond)
	// Stop the AsyncGossiper
	p.Stop()
	// Test that the AsyncGossiper stops within a short duration
	require.Eventually(t, func() bool {
		return !p.running.Load()
	}, time.Second, 10*time.Millisecond)
}

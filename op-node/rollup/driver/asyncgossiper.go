package driver

import (
	"context"
	"sync/atomic"

	"github.com/ethereum-optimism/optimism/op-service/eth"
)

// AsyncGossiper is a component for gossiping a payload while running the derivation pipeline
// the gossiping is done in a separate goroutine, so that the derivation pipeline can continue without waiting for the gossiping to finish
// it also maintains the currently gossiping payload, so it can be checked against other found payloads
type AsyncGossiper struct {
	running atomic.Bool
	// channel to add new payloads to gossip
	set chan *eth.ExecutionPayload
	// channel to request getting the currently gossiping payload
	get chan chan *eth.ExecutionPayload
	// channel to request clearing the currently gossiping payload
	clear chan struct{}

	hasPayload     atomic.Bool
	currentPayload *eth.ExecutionPayload
	net            Network
}

func NewAsyncGossiper(net Network) *AsyncGossiper {
	return &AsyncGossiper{
		running: atomic.Bool{},
		set:     make(chan *eth.ExecutionPayload, 1),
		get:     make(chan chan *eth.ExecutionPayload),
		clear:   make(chan struct{}),

		hasPayload:     atomic.Bool{},
		currentPayload: nil,
		net:            net,
	}
}

// Gossip is an exposed, syncronous function to send a payload to be gossiped into the pregossiper
func (p *AsyncGossiper) Gossip(payload *eth.ExecutionPayload) {
	// send the payload to the newPayloads channel. this will block until the payload can be sent
	p.set <- payload
}

// Get is an exposed, syncronous function to get the currently gossiping payload from the async pregossiper
func (p *AsyncGossiper) Get() *eth.ExecutionPayload {
	c := make(chan *eth.ExecutionPayload)
	// send the channel as a request. this will block until sent
	p.get <- c
	// return the response payload. this will block until received
	return <-c
}

// HasPayload is an exposed, syncronous function to check if the pregossiper is currently holding a payload
func (p *AsyncGossiper) HasPayload() bool {
	return p.hasPayload.Load()
}

// Clear is an exposed, syncronous function to clear the currently gossiping payload from the async pregossiper's state
func (p *AsyncGossiper) Clear() {
	// send the signal to the clearPayload channel. this will block until the payload can be sent
	p.clear <- struct{}{}
}

// Start starts the pregossiper's gossiping loop on a separate goroutine
// each behavior of the gossiping loop is handled by a select case on a channel, plus an internal handler function call
func (p *AsyncGossiper) Start(ctx context.Context) {
	// if the gossiping is already running, return
	if p.running.Load() {
		return
	}
	p.running.Store(true)
	// else, start the handling loop
	go func() {
		defer p.running.Store(false)
		for {
			select {
			// new payloads to be gossiped are found in the `set` channel
			case payload := <-p.set:
				p.setPayload(payload)
				p.gossip(ctx)
			// requests to get the current payload are found in the `get` channel
			case c := <-p.get:
				p.getPayload(c)
			// requests to clear the current payload are found in the `clear` channel
			case <-p.clear:
				p.clearPayload()
			// if the context is done, return
			case <-ctx.Done():
				return
			}
		}
	}()
}

// setPayload is the internal handler function for setting the current payload
// payload is the payload to set as the current payload
func (p *AsyncGossiper) setPayload(payload *eth.ExecutionPayload) {
	p.currentPayload = payload
	p.hasPayload.Store(true)
}

// gossip is the internal handler function for gossiping the current payload
// it is called by the gossiping loop when a new payload is set
func (p *AsyncGossiper) gossip(ctx context.Context) {
	p.net.PublishL2Payload(ctx, p.currentPayload)
}

// getPayload is the internal handler function for getting the current payload
// c is the channel the caller expects to receive the payload on
func (p *AsyncGossiper) getPayload(c chan *eth.ExecutionPayload) {
	c <- p.currentPayload
}

// clearPayload is the internal handler function for clearing the current payload
func (p *AsyncGossiper) clearPayload() {
	p.currentPayload = nil
	p.hasPayload.Store(false)
}

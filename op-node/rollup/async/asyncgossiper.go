package async

import (
	"context"
	"sync/atomic"

	"github.com/ethereum/go-ethereum/log"

	"github.com/ethereum-optimism/optimism/op-service/eth"
)

// AsyncGossiper is a component that stores and gossips a single payload at a time
// it uses a separate goroutine to handle gossiping the payload asynchronously
// the payload can be accessed by the Get function to be reused when the payload was gossiped but not inserted
// exposed functions are synchronous, and block until the async routine is able to start handling the request

type AsyncGossiper struct {
	running atomic.Bool
	// channel to add new payloads to gossip
	set chan *eth.ExecutionPayload
	// channel to request getting the currently gossiping payload
	get chan chan *eth.ExecutionPayload
	// channel to request clearing the currently gossiping payload
	clear chan struct{}

	currentPayload *eth.ExecutionPayload
	net            Network
	log            log.Logger
	metrics        Metrics
}

// To avoid import cycles, we define a new Network interface here
// this interface is compatable with driver.Network
type Network interface {
	PublishL2Payload(ctx context.Context, payload *eth.ExecutionPayload) error
}

// To avoid import cycles, we define a new Metrics interface here
// this interface is compatable with driver.Metrics
type Metrics interface {
	RecordPublishingError()
}

func NewAsyncGossiper(net Network, log log.Logger, metrics Metrics) *AsyncGossiper {
	return &AsyncGossiper{
		running: atomic.Bool{},
		set:     make(chan *eth.ExecutionPayload, 1),
		get:     make(chan chan *eth.ExecutionPayload),
		clear:   make(chan struct{}),

		currentPayload: nil,
		net:            net,
		log:            log,
		metrics:        metrics,
	}
}

// Gossip is a synchronous function to store and gossip a payload
// it blocks until the payload can be taken by the async routine
func (p *AsyncGossiper) Gossip(payload *eth.ExecutionPayload) {
	p.set <- payload
}

// Get is a synchronous function to get the currently held payload
// it blocks until the async routine is able to return the payload
func (p *AsyncGossiper) Get() *eth.ExecutionPayload {
	c := make(chan *eth.ExecutionPayload)
	p.get <- c
	return <-c
}

// Clear is a synchronous function to clear the currently gossiping payload
// it blocks until the signal to clear is picked up by the async routine
func (p *AsyncGossiper) Clear() {
	p.clear <- struct{}{}
}

// Start starts the AsyncGossiper's gossiping loop on a separate goroutine
// each behavior of the loop is handled by a select case on a channel, plus an internal handler function call
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
				p.gossip(ctx, payload)
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

// gossip is the internal handler function for gossiping the current payload
// and storing the payload in the async AsyncGossiper's state
// it is called by the Start loop when a new payload is set
// the payload is only stored if the publish is successful
func (p *AsyncGossiper) gossip(ctx context.Context, payload *eth.ExecutionPayload) {
	if err := p.net.PublishL2Payload(ctx, payload); err == nil {
		p.currentPayload = payload
	} else {
		p.log.Warn("failed to publish newly created block", "id", payload.ID(), "err", err)
		p.metrics.RecordPublishingError()
	}
}

// getPayload is the internal handler function for getting the current payload
// c is the channel the caller expects to receive the payload on
func (p *AsyncGossiper) getPayload(c chan *eth.ExecutionPayload) {
	c <- p.currentPayload
}

// clearPayload is the internal handler function for clearing the current payload
func (p *AsyncGossiper) clearPayload() {
	p.currentPayload = nil
}

package service

import (
	"fmt"
	"sync"
	"time"

	cfg "github.com/Manta-Network/manta-fp/bbn-fp/config"
	"github.com/Manta-Network/manta-fp/clientcontroller"
	"github.com/Manta-Network/manta-fp/metrics"
	"github.com/Manta-Network/manta-fp/types"

	"github.com/avast/retry-go/v4"
	"go.uber.org/atomic"
	"go.uber.org/zap"
)

var (
	RtyAttNum = uint(5)
	RtyAtt    = retry.Attempts(RtyAttNum)
	RtyDel    = retry.Delay(time.Millisecond * 400)
	RtyErr    = retry.LastErrorOnly(true)
)

const (
	// TODO: Maybe configurable?
	maxFailedCycles = 20
)

type skipHeightRequest struct {
	height uint64
	resp   chan *skipHeightResponse
}

type skipHeightResponse struct {
	err error
}

type ChainPoller struct {
	isStarted *atomic.Bool
	wg        sync.WaitGroup
	quit      chan struct{}

	cc             clientcontroller.ClientController
	cfg            *cfg.ChainPollerConfig
	metrics        *metrics.FpMetrics
	blockInfoChan  chan *types.BlockInfo
	skipHeightChan chan *skipHeightRequest
	nextHeight     uint64
	logger         *zap.Logger
}

func NewChainPoller(
	logger *zap.Logger,
	cfg *cfg.ChainPollerConfig,
	cc clientcontroller.ClientController,
	metrics *metrics.FpMetrics,
) *ChainPoller {
	return &ChainPoller{
		isStarted:      atomic.NewBool(false),
		logger:         logger,
		cfg:            cfg,
		cc:             cc,
		metrics:        metrics,
		blockInfoChan:  make(chan *types.BlockInfo, cfg.BufferSize),
		skipHeightChan: make(chan *skipHeightRequest),
		quit:           make(chan struct{}),
	}
}

func (cp *ChainPoller) Start(startHeight uint64) error {
	if cp.isStarted.Swap(true) {
		return fmt.Errorf("the poller is already started")
	}

	cp.logger.Info("starting the chain poller")

	cp.nextHeight = startHeight

	cp.wg.Add(1)

	go cp.pollChain()

	cp.metrics.RecordPollerStartingHeight(startHeight)
	cp.logger.Info("the chain poller is successfully started")

	return nil
}

func (cp *ChainPoller) Stop() error {
	if !cp.isStarted.Swap(false) {
		return fmt.Errorf("the chain poller has already stopped")
	}

	cp.logger.Info("stopping the chain poller")
	err := cp.cc.Close()
	if err != nil {
		return err
	}
	close(cp.quit)
	cp.wg.Wait()

	cp.logger.Info("the chain poller is successfully stopped")

	return nil
}

func (cp *ChainPoller) IsRunning() bool {
	return cp.isStarted.Load()
}

// Return read only channel for incoming blocks
// TODO: Handle the case when there is more than one consumer. Currently with more than
// one consumer blocks most probably will be received out of order to those consumers.
func (cp *ChainPoller) GetBlockInfoChan() <-chan *types.BlockInfo {
	return cp.blockInfoChan
}

func (cp *ChainPoller) blockWithRetry(height uint64) (*types.BlockInfo, error) {
	var (
		block *types.BlockInfo
		//err   error
	)
	//if err := retry.Do(func() error {
	//	block, err = cp.cc.QueryBlock(height)
	//	if err != nil {
	//		return err
	//	}
	//	return nil
	//}, RtyAtt, RtyDel, RtyErr, retry.OnRetry(func(n uint, err error) {
	//	cp.logger.Debug(
	//		"failed to query the consumer chain for the latest block",
	//		zap.Uint("attempt", n+1),
	//		zap.Uint("max_attempts", RtyAttNum),
	//		zap.Uint64("height", height),
	//		zap.Error(err),
	//	)
	//})); err != nil {
	//	return nil, err
	//}

	return block, nil
}

// waitForActivation waits until BTC staking is activated
func (cp *ChainPoller) waitForActivation() {
	// ensure that the startHeight is no lower than the activated height
	for {
		select {
		case <-time.After(cp.cfg.PollInterval):
			activatedHeight, err := cp.cc.QueryActivatedHeight()
			if err != nil {
				cp.logger.Debug("failed to query the consumer chain for the activated height", zap.Error(err))
			} else {
				if cp.nextHeight < activatedHeight {
					cp.nextHeight = activatedHeight
				}
				return
			}

		case <-cp.quit:
			return
		}
	}
}

func (cp *ChainPoller) pollChain() {
	defer cp.wg.Done()

	cp.waitForActivation()

	var failedCycles uint32

	for {
		select {
		case <-time.After(cp.cfg.PollInterval):
			// TODO: Handlig of request cancellation, as otherwise shutdown will be blocked
			// until request is finished
			blockToRetrieve := cp.nextHeight
			block, err := cp.blockWithRetry(blockToRetrieve)
			if err != nil {
				failedCycles++
				cp.logger.Debug(
					"failed to query the consumer chain for the block",
					zap.Uint32("current_failures", failedCycles),
					zap.Uint64("block_to_retrieve", blockToRetrieve),
					zap.Error(err),
				)
			} else {
				// no error and we got the header we wanted to get, bump the state and push
				// notification about data
				cp.nextHeight = blockToRetrieve + 1
				failedCycles = 0
				cp.metrics.RecordLastPolledHeight(block.Height)

				cp.logger.Info("the poller retrieved the block from the consumer chain",
					zap.Uint64("height", block.Height))

				// push the data to the channel
				// Note: if the consumer is too slow -- the buffer is full
				// the channel will block, and we will stop retrieving data from the node
				cp.blockInfoChan <- block
			}

			if failedCycles > maxFailedCycles {
				cp.logger.Fatal("the poller has reached the max failed cycles, exiting")
			}
		case req := <-cp.skipHeightChan:
			// no need to skip heights if the target height is not higher
			// than the next height to retrieve
			targetHeight := req.height
			if targetHeight <= cp.nextHeight {
				resp := &skipHeightResponse{
					err: fmt.Errorf(
						"the target height %d is not higher than the next height %d to retrieve",
						targetHeight, cp.nextHeight)}
				req.resp <- resp
				continue
			}

			// drain blocks that can be skipped from blockInfoChan
			cp.clearChanBufferUpToHeight(targetHeight)

			// set the next height to the skip height
			cp.nextHeight = targetHeight

			cp.logger.Debug("the poller has skipped height(s)",
				zap.Uint64("next_height", req.height))

			req.resp <- &skipHeightResponse{}

		case <-cp.quit:
			return
		}
	}
}

func (cp *ChainPoller) SkipToHeight(height uint64) error {
	if !cp.IsRunning() {
		return fmt.Errorf("the chain poller is stopped")
	}

	respChan := make(chan *skipHeightResponse, 1)

	// this handles the case when the poller is stopped before the
	// skip height request is sent
	select {
	case <-cp.quit:
		return fmt.Errorf("the chain poller is stopped")
	case cp.skipHeightChan <- &skipHeightRequest{height: height, resp: respChan}:
	}

	// this handles the case when the poller is stopped before
	// the skip height request is returned
	select {
	case <-cp.quit:
		return fmt.Errorf("the chain poller is stopped")
	case resp := <-respChan:
		return resp.err
	}
}

func (cp *ChainPoller) NextHeight() uint64 {
	return cp.nextHeight
}

func (cp *ChainPoller) clearChanBufferUpToHeight(upToHeight uint64) {
	for len(cp.blockInfoChan) > 0 {
		block := <-cp.blockInfoChan
		if block.Height+1 >= upToHeight {
			break
		}
	}
}

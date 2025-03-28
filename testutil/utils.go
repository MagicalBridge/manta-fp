package testutil

import (
	"math/rand"
	"testing"

	"github.com/Manta-Network/manta-fp/testutil/mocks"
	"github.com/Manta-Network/manta-fp/types"

	sdkmath "cosmossdk.io/math"
	"github.com/golang/mock/gomock"
)

const TestPubRandNum = 25

func ZeroCommissionRate() *sdkmath.LegacyDec {
	zeroCom := sdkmath.LegacyZeroDec()
	return &zeroCom
}

func PrepareMockedClientController(t *testing.T, r *rand.Rand, startHeight, currentHeight, finalityActivationBlkHeight uint64) *mocks.MockClientController {
	ctl := gomock.NewController(t)
	mockClientController := mocks.NewMockClientController(ctl)

	for i := startHeight; i <= currentHeight; i++ {
		resBlock := &types.BlockInfo{
			Height: currentHeight,
			Hash:   GenRandomByteArray(r, 32),
		}
		mockClientController.EXPECT().QueryBlock(i).Return(resBlock, nil).AnyTimes()
	}

	currentBlockRes := &types.BlockInfo{
		Height: currentHeight,
		Hash:   GenRandomByteArray(r, 32),
	}

	mockClientController.EXPECT().Close().Return(nil).AnyTimes()
	mockClientController.EXPECT().QueryBestBlock().Return(currentBlockRes, nil).AnyTimes()
	mockClientController.EXPECT().QueryActivatedHeight().Return(uint64(1), nil).AnyTimes()
	mockClientController.EXPECT().QueryFinalityActivationBlockHeight().Return(finalityActivationBlkHeight, nil).AnyTimes()

	return mockClientController
}

package evm

import (
	"math/big"
	"os"
	"testing"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/utils/logging"
	"github.com/ava-labs/awm-relayer/config"
	"github.com/ava-labs/awm-relayer/database"
	mock_ethclient "github.com/ava-labs/awm-relayer/vms/evm/mocks"
	"github.com/ava-labs/subnet-evm/core/types"
	"github.com/ava-labs/subnet-evm/ethclient"
	"github.com/ava-labs/subnet-evm/interfaces"
	"github.com/ava-labs/subnet-evm/x/warp"
	"github.com/ethereum/go-ethereum/common"
	"go.uber.org/mock/gomock"
)

func makeSubscriberWithMockEthClient(t *testing.T) (subscriber, *mock_ethclient.MockClient) {
	sourceSubnet := config.SourceSubnet{
		SubnetID:          "2TGBXcnwx5PqiXWiqxAKUaNSqDguXNh1mxnp82jui68hxJSZAx",
		ChainID:           "S4mMqUXe7vHsGiRAma6bv3CKnyaLssyAxmQ2KvFpX1KEvfFCD",
		VM:                config.EVM.String(),
		APINodeHost:       "127.0.0.1",
		APINodePort:       9650,
		EncryptConnection: false,
		RPCEndpoint:       "https://subnets.avax.network/mysubnet/rpc",
	}

	logger := logging.NewLogger(
		"awm-relayer-test",
		logging.NewWrappedCore(
			logging.Info,
			os.Stdout,
			logging.JSON.ConsoleEncoder(),
		),
	)

	subnetId, err := ids.FromString(sourceSubnet.ChainID)
	if err != nil {
		t.Fatalf("Failed to create subnet ID")
	}

	db, err := database.NewJSONFileStorage(logger, t.TempDir(), []ids.ID{subnetId})
	if err != nil {
		t.Fatalf("Failed to create JSON file storage")
	}

	mockEthClient := mock_ethclient.NewMockClient(gomock.NewController(t))
	stockSubscriber := NewSubscriber(logger, sourceSubnet, db)
	subscriberUnderTest := subscriber{
		nodeWSURL:  stockSubscriber.nodeWSURL,
		nodeRPCURL: stockSubscriber.nodeRPCURL,
		chainID:    stockSubscriber.chainID,
		logsChan:   stockSubscriber.logsChan,
		evmLog:     stockSubscriber.evmLog,
		logger:     stockSubscriber.logger,
		db:         stockSubscriber.db,
		dial:       func(_url string) (ethclient.Client, error) { return mockEthClient, nil },
	}

	return subscriberUnderTest, mockEthClient
}

func TestProcessFromHeight(t *testing.T) {
	testCases := []struct {
		latest int64
		input  int64
	}{
		{
			latest: 1000,
			input:  800,
		},
		{
			latest: 1000,
			input:  700,
		},
		{
			latest: 19642,
			input:  751,
		},
		{
			latest: 96,
			input:  41,
		},
	}

	expectFilterLogs := func(
		mock *mock_ethclient.MockClient,
		fromBlock int64,
		toBlock int64,
	) {
		mock.EXPECT().FilterLogs(
			gomock.Any(),
			interfaces.FilterQuery{
				Topics: [][]common.Hash{
					{warp.WarpABI.Events["SendWarpMessage"].ID},
					{},
					{},
				},
				Addresses: []common.Address{
					warp.ContractAddress,
				},
				FromBlock: big.NewInt(fromBlock),
				ToBlock:   big.NewInt(toBlock),
			},
		).Return([]types.Log{}, nil).Times(1)
	}

	min := func(a, b int64) int64 {
		if a < b {
			return a
		} else {
			return b
		}
	}

	for _, tc := range testCases {
		subscriberUnderTest, mockEthClient := makeSubscriberWithMockEthClient(t)

		mockEthClient.
			EXPECT().
			BlockNumber(gomock.Any()).
			Return(uint64(tc.latest), nil).
			Times(1)

		for i := tc.input; i < tc.latest; i += MaxBlocksPerRequest + 1 {
			expectFilterLogs(
				mockEthClient,
				i,
				min(i+MaxBlocksPerRequest, tc.latest),
			)
		}

		subscriberUnderTest.ProcessFromHeight(big.NewInt(tc.input))
	}
}

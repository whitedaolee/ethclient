package gnosissafe

import (
	"errors"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ivanzzeth/ethclient"
	"github.com/ivanzzeth/ethclient/message"
)

var _ SafeTxDeliverer = &SafeTxDelivererByEthClient{}

// SafeTxDeliverer dispatches requests to the underlying layer, where each request wraps a call to Safe's execTransaction.
// TODO: Deliverer should validate the request's gas to ensure it meets the minimum requirement for successful Safe contract execution (avoiding reverts).
type SafeTxDeliverer interface {
	Deliver(req *message.Request, safeNonce uint64) error
}

type SafeTxDelivererByEthClient struct {
	ethClient                         *ethclient.Client
	clientSendTxAddr                  common.Address
	addrToCaller                      sync.Map
	defaultSafelContractCallerCreator SafelContractCallerCreator
}

type DelivererByEthClientOption interface {
	apply(*SafeTxDelivererByEthClient)
}

type optionFunc func(*SafeTxDelivererByEthClient)

func (f optionFunc) apply(deliverer *SafeTxDelivererByEthClient) {
	f(deliverer)
}

func WithDefaultSafelContractCallerCreator(creator SafelContractCallerCreator) optionFunc {

	return func(deliverer *SafeTxDelivererByEthClient) {
		deliverer.defaultSafelContractCallerCreator = creator
	}
}

func NewSafeTxDelivererByEthClient(ethClient *ethclient.Client, clientSendTxAddr common.Address, options ...DelivererByEthClientOption) SafeTxDeliverer {
	out := &SafeTxDelivererByEthClient{
		ethClient:                         ethClient,
		clientSendTxAddr:                  clientSendTxAddr,
		defaultSafelContractCallerCreator: NewDefaultSafelContractCallerCreator,
	}

	for _, option := range options {
		option.apply(out)
	}
	return out
}

func (deliverer *SafeTxDelivererByEthClient) Deliver(req *message.Request, safeNonce uint64) (err error) {

	if req.From != deliverer.clientSendTxAddr {
		return errors.New("from address do not match")
	}

	value, ok := deliverer.addrToCaller.Load(*req.To)
	if !ok {
		value, err = deliverer.defaultSafelContractCallerCreator(*req.To, deliverer.ethClient.Client)
		if err != nil {
			return err
		}
		deliverer.addrToCaller.Store(*req.To, value)
	}
	safelContractCaller := value.(SafelContractCaller)

	nonceInChain, err := safelContractCaller.GetNonce()
	if err != nil {
		return err
	}

	if nonceInChain < safeNonce {
		req.AfterMsg = message.GenerateMessageIdByAddressAndNonce(*req.To, int64(safeNonce-1))
		log.Debug("GenerateMessageIdByAddressAndNonce for MSG : ", "ID", req.Id(), "afterMSG", req.AfterMsg)
	} else if nonceInChain > safeNonce {
		return errors.New("safeNonce is invalid")
	}

	// sync schedule
	deliverer.ethClient.ScheduleMsg(req)
	log.Debug("deliverer sync schedule Msg : ", req.Id().Hex())
	return nil
}

package bob

import (
	"errors"
	"fmt"
	"time"

	ethcommon "github.com/ethereum/go-ethereum/common"

	"github.com/noot/atomic-swap/common"
	mcrypto "github.com/noot/atomic-swap/crypto/monero"
	"github.com/noot/atomic-swap/net"
	pcommon "github.com/noot/atomic-swap/protocol"
	pswap "github.com/noot/atomic-swap/protocol/swap"
	"github.com/noot/atomic-swap/swap-contract"
)

// HandleProtocolMessage is called by the network to handle an incoming message.
// If the message received is not the expected type for the point in the protocol we're at,
// this function will return an error.
func (s *swapState) HandleProtocolMessage(msg net.Message) (net.Message, bool, error) {
	s.Lock()
	defer s.Unlock()

	if s.ctx.Err() != nil {
		return nil, true, fmt.Errorf("protocol exited: %w", s.ctx.Err())
	}

	if err := s.checkMessageType(msg); err != nil {
		return nil, true, err
	}

	switch msg := msg.(type) {
	case *net.SendKeysMessage:
		if err := s.handleSendKeysMessage(msg); err != nil {
			return nil, true, err
		}

		return nil, false, nil
	case *net.NotifyContractDeployed:
		out, err := s.handleNotifyContractDeployed(msg)
		if err != nil {
			return nil, true, err
		}

		return out, false, nil
	case *net.NotifyReady:
		log.Debug("contract ready, attempting to claim funds...")
		close(s.readyCh)

		// contract ready, let's claim our ether
		txHash, err := s.claimFunds()
		if err != nil {
			return nil, true, fmt.Errorf("failed to redeem ether: %w", err)
		}

		log.Debug("funds claimed!!")
		out := &net.NotifyClaimed{
			TxHash: txHash.String(),
		}

		s.info.SetStatus(pswap.Success)
		return out, true, nil
	case *net.NotifyRefund:
		// generate monero wallet, regaining control over locked funds
		addr, err := s.handleRefund(msg.TxHash)
		if err != nil {
			return nil, false, err
		}

		s.info.SetStatus(pswap.Refunded)
		log.Infof("regained control over monero account %s", addr)
		return nil, true, nil
	default:
		return nil, true, errors.New("unexpected message type")
	}
}

func (s *swapState) checkMessageType(msg net.Message) error {
	// Alice might refund anytime before t0 or after t1, so we should allow this.
	if _, ok := msg.(*net.NotifyRefund); ok {
		return nil
	}

	if msg.Type() != s.nextExpectedMessage.Type() {
		return errors.New("received unexpected message")
	}

	return nil
}

func (s *swapState) handleNotifyContractDeployed(msg *net.NotifyContractDeployed) (net.Message, error) {
	if msg.Address == "" {
		return nil, errMissingAddress
	}

	log.Infof("got Swap contract address! address=%s", msg.Address)

	if err := s.setContract(ethcommon.HexToAddress(msg.Address)); err != nil {
		return nil, fmt.Errorf("failed to instantiate contract instance: %w", err)
	}

	fp := fmt.Sprintf("%s/%d/contractaddress", s.bob.basepath, s.ID())
	if err := common.WriteContractAddressToFile(fp, msg.Address); err != nil {
		return nil, fmt.Errorf("failed to write contract address to file: %w", err)
	}

	if err := s.checkContract(); err != nil {
		return nil, err
	}

	addrAB, err := s.lockFunds(common.MoneroToPiconero(s.info.ProvidedAmount()))
	if err != nil {
		return nil, fmt.Errorf("failed to lock funds: %w", err)
	}

	out := &net.NotifyXMRLock{
		Address: string(addrAB),
	}

	// set t0 and t1
	if err := s.setTimeouts(); err != nil {
		return nil, err
	}

	go func() {
		until := time.Until(s.t0)

		log.Debugf("time until t0: %vs", until.Seconds())

		select {
		case <-s.ctx.Done():
			return
		case <-time.After(until + time.Second):
			// we can now call Claim()
			txHash, err := s.claimFunds()
			if err != nil {
				log.Errorf("failed to claim: err=%s", err)
				// TODO: retry claim, depending on error
				return
			}

			log.Debug("funds claimed!")
			s.info.SetStatus(pswap.Success)

			// send *net.NotifyClaimed
			if err := s.bob.net.SendSwapMessage(&net.NotifyClaimed{
				TxHash: txHash.String(),
			}); err != nil {
				log.Errorf("failed to send NotifyClaimed message: err=%s", err)
			}
		case <-s.readyCh:
			return
		}
	}()

	s.nextExpectedMessage = &net.NotifyReady{}
	return out, nil
}

func (s *swapState) handleSendKeysMessage(msg *net.SendKeysMessage) error {
	if msg.PublicSpendKey == "" || msg.PublicViewKey == "" {
		return errMissingKeys
	}

	log.Debug("got Alice's public keys")

	kp, err := mcrypto.NewPublicKeyPairFromHex(msg.PublicSpendKey, msg.PublicViewKey)
	if err != nil {
		return fmt.Errorf("failed to generate Alice's public keys: %w", err)
	}

	// verify counterparty's DLEq proof and ensure the resulting secp256k1 key is correct
	secp256k1Pub, err := pcommon.VerifyKeysAndProof(msg.DLEqProof, msg.Secp256k1PublicKey)
	if err != nil {
		return err
	}

	s.setAlicePublicKeys(kp, secp256k1Pub)
	s.nextExpectedMessage = &net.NotifyContractDeployed{}
	return nil
}

func (s *swapState) handleRefund(txHash string) (mcrypto.Address, error) {
	receipt, err := s.bob.ethClient.TransactionReceipt(s.ctx, ethcommon.HexToHash(txHash))
	if err != nil {
		return "", err
	}

	if len(receipt.Logs) == 0 {
		return "", errors.New("claim transaction has no logs")
	}

	sa, err := swap.GetSecretFromLog(receipt.Logs[0], "Refunded")
	if err != nil {
		return "", err
	}

	return s.reclaimMonero(sa)
}

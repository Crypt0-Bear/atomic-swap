package alice

import (
	"errors"
	"fmt"
	"time"

	"github.com/noot/atomic-swap/common"
	mcrypto "github.com/noot/atomic-swap/crypto/monero"
	"github.com/noot/atomic-swap/monero"
	"github.com/noot/atomic-swap/net"
	pcommon "github.com/noot/atomic-swap/protocol"
	pswap "github.com/noot/atomic-swap/protocol/swap"
	"github.com/noot/atomic-swap/swap-contract"

	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/fatih/color" //nolint:misspell
)

// HandleProtocolMessage is called by the network to handle an incoming message.
// If the message received is not the expected type for the point in the protocol we're at,
// this function will return an error.
func (s *swapState) HandleProtocolMessage(msg net.Message) (net.Message, bool, error) {
	s.Lock()
	defer s.Unlock()

	if err := s.checkMessageType(msg); err != nil {
		return nil, true, err
	}

	switch msg := msg.(type) {
	case *net.SendKeysMessage:
		resp, err := s.handleSendKeysMessage(msg)
		if err != nil {
			return nil, true, err
		}

		return resp, false, nil
	case *net.NotifyXMRLock:
		out, err := s.handleNotifyXMRLock(msg)
		if err != nil {
			return nil, true, err
		}

		return out, false, nil
	case *net.NotifyClaimed:
		address, err := s.handleNotifyClaimed(msg.TxHash)
		if err != nil {
			log.Error("failed to create monero address: err=", err)
			return nil, true, err
		}

		close(s.claimedCh)
		log.Info("successfully created monero wallet from our secrets: address=", address)
		s.info.SetStatus(pswap.Success)
		return nil, true, nil
	default:
		return nil, false, errors.New("unexpected message type")
	}
}

func (s *swapState) checkMessageType(msg net.Message) error {
	if msg.Type() != s.nextExpectedMessage.Type() {
		return errors.New("received unexpected message")
	}

	return nil
}

func (s *swapState) handleSendKeysMessage(msg *net.SendKeysMessage) (net.Message, error) {
	// TODO: get user to confirm amount they will receive!!
	s.info.SetReceivedAmount(msg.ProvidedAmount)
	log.Infof(color.New(color.Bold).Sprintf("you will be receiving %v XMR", msg.ProvidedAmount))

	exchangeRate := msg.ProvidedAmount / s.info.ProvidedAmount()
	s.info.SetExchangeRate(common.ExchangeRate(exchangeRate))

	if msg.PublicSpendKey == "" || msg.PrivateViewKey == "" {
		return nil, errMissingKeys
	}

	if msg.EthAddress == "" {
		return nil, errMissingAddress
	}

	vk, err := mcrypto.NewPrivateViewKeyFromHex(msg.PrivateViewKey)
	if err != nil {
		return nil, fmt.Errorf("failed to generate Bob's private view keys: %w", err)
	}

	s.bobAddress = ethcommon.HexToAddress(msg.EthAddress)

	log.Debugf("got Bob's keys and address: address=%s", s.bobAddress)

	sk, err := mcrypto.NewPublicKeyFromHex(msg.PublicSpendKey)
	if err != nil {
		return nil, fmt.Errorf("failed to generate Bob's public spend key: %w", err)
	}

	// verify counterparty's DLEq proof and ensure the resulting secp256k1 key is correct
	secp256k1Pub, err := pcommon.VerifyKeysAndProof(msg.DLEqProof, msg.Secp256k1PublicKey)
	if err != nil {
		return nil, err
	}

	s.setBobKeys(sk, vk, secp256k1Pub)
	address, err := s.deployAndLockETH(s.providedAmountInWei())
	if err != nil {
		return nil, fmt.Errorf("failed to deploy contract: %w", err)
	}

	log.Info("deployed Swap contract, waiting for XMR to be locked: contract address=", address)

	// set t0 and t1
	// TODO: these sometimes fail with "attempting to unmarshall an empty string while arguments are expected"
	if err := s.setTimeouts(); err != nil {
		return nil, err
	}

	// start goroutine to check that Bob locks before t_0
	go func() {
		const timeoutBuffer = time.Minute * 5
		until := time.Until(s.t0)

		select {
		case <-s.ctx.Done():
			return
		case <-time.After(until - timeoutBuffer):
			// Bob hasn't locked yet, let's call refund
			txhash, err := s.refund()
			if err != nil {
				log.Errorf("failed to refund: err=%s", err)
				return
			}

			log.Infof("got our ETH back: tx hash=%s", txhash)

			// send NotifyRefund msg
			if err := s.alice.net.SendSwapMessage(&net.NotifyRefund{
				TxHash: txhash.String(),
			}); err != nil {
				log.Errorf("failed to send refund message: err=%s", err)
			}
		case <-s.xmrLockedCh:
			return
		}

	}()

	s.nextExpectedMessage = &net.NotifyXMRLock{}

	out := &net.NotifyContractDeployed{
		Address: address.String(),
	}

	return out, nil
}

func (s *swapState) handleNotifyXMRLock(msg *net.NotifyXMRLock) (net.Message, error) {
	if msg.Address == "" {
		return nil, errors.New("got empty address for locked XMR")
	}
	// check that XMR was locked in expected account, and confirm amount
	vk := mcrypto.SumPrivateViewKeys(s.bobPrivateViewKey, s.privkeys.ViewKey())
	sk := mcrypto.SumPublicKeys(s.bobPublicSpendKey, s.pubkeys.SpendKey())
	kp := mcrypto.NewPublicKeyPair(sk, vk.Public())

	if msg.Address != string(kp.Address(s.alice.env)) {
		return nil, fmt.Errorf("address received in message does not match expected address")
	}

	t := time.Now().Format("2006-Jan-2-15:04:05")
	walletName := fmt.Sprintf("alice-viewonly-wallet-%s", t)
	log.Debugf("generating view-only wallet to check funds: %s", walletName)
	if err := s.alice.client.GenerateViewOnlyWalletFromKeys(vk, kp.Address(s.alice.env), walletName, ""); err != nil {
		return nil, fmt.Errorf("failed to generate view-only wallet to verify locked XMR: %w", err)
	}

	log.Debugf("generated view-only wallet to check funds: %s", walletName)

	if s.alice.env != common.Development {
		// wait for 2 new blocks, otherwise balance might be 0
		// TODO: check transaction hash
		if err := monero.WaitForBlocks(s.alice.client); err != nil {
			return nil, err
		}

		if err := monero.WaitForBlocks(s.alice.client); err != nil {
			return nil, err
		}
	}

	if err := s.alice.client.Refresh(); err != nil {
		return nil, fmt.Errorf("failed to refresh client: %w", err)
	}

	accounts, err := s.alice.client.GetAccounts()
	if err != nil {
		return nil, fmt.Errorf("failed to get accounts: %w", err)
	}

	var (
		balance *monero.GetBalanceResponse
	)

	for i, acc := range accounts.SubaddressAccounts {
		addr, ok := acc["base_address"].(string)
		if !ok {
			panic("address is not a string!")
		}

		if mcrypto.Address(addr) == kp.Address(s.alice.env) {
			balance, err = s.alice.client.GetBalance(uint(i))
			if err != nil {
				return nil, fmt.Errorf("failed to get balance: %w", err)
			}

			break
		}
	}

	if balance == nil {
		return nil, fmt.Errorf("failed to find account with address %s", kp.Address(s.alice.env))
	}

	log.Debugf("checking locked wallet, address=%s balance=%v", kp.Address(s.alice.env), balance.Balance)

	// TODO: also check that the balance isn't unlocked only after an unreasonable amount of blocks
	if balance.Balance < float64(s.receivedAmountInPiconero()) {
		return nil, fmt.Errorf("locked XMR amount is less than expected: got %v, expected %v",
			balance.Balance, float64(s.receivedAmountInPiconero()))
	}

	if err := s.alice.client.CloseWallet(); err != nil {
		return nil, fmt.Errorf("failed to close wallet: %w", err)
	}

	close(s.xmrLockedCh)

	if err := s.ready(); err != nil {
		return nil, fmt.Errorf("failed to call Ready: %w", err)
	}

	log.Debug("set swap.IsReady to true")

	if err := s.setTimeouts(); err != nil {
		return nil, fmt.Errorf("failed to set timeouts: %w", err)
	}

	go func() {
		until := time.Until(s.t1)

		select {
		case <-s.ctx.Done():
			return
		case <-time.After(until + time.Second):
			// Bob hasn't claimed, and we're after t_1. let's call Refund
			txhash, err := s.refund()
			if err != nil {
				log.Errorf("failed to refund: err=%s", err)
				return
			}

			log.Infof("got our ETH back: tx hash=%s", txhash)

			// send NotifyRefund msg
			if err = s.alice.net.SendSwapMessage(&net.NotifyRefund{
				TxHash: txhash.String(),
			}); err != nil {
				log.Errorf("failed to send refund message: err=%s", err)
			}
		case <-s.claimedCh:
			return
		}
	}()

	s.nextExpectedMessage = &net.NotifyClaimed{}
	return &net.NotifyReady{}, nil
}

// handleNotifyClaimed handles Bob's reveal after he calls Claim().
// it calls `createMoneroWallet` to create Alice's wallet, allowing her to own the XMR.
func (s *swapState) handleNotifyClaimed(txHash string) (mcrypto.Address, error) {
	receipt, ok := common.WaitForReceipt(s.ctx, s.alice.ethClient, ethcommon.HexToHash(txHash))
	if !ok {
		return "", errors.New("failed check Claim transaction receipt")
	}

	if len(receipt.Logs) == 0 {
		return "", errors.New("claim transaction has no logs")
	}

	skB, err := swap.GetSecretFromLog(receipt.Logs[0], "Claimed")
	if err != nil {
		return "", fmt.Errorf("failed to get secret from log: %w", err)
	}

	return s.claimMonero(skB)
}

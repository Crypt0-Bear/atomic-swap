package swap

import (
	"errors"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	ethtypes "github.com/ethereum/go-ethereum/core/types"

	"github.com/noot/atomic-swap/common"
	mcrypto "github.com/noot/atomic-swap/crypto/monero"
)

// GetSecretFromLog returns the secret from a Claimed or Refunded log
func GetSecretFromLog(log *ethtypes.Log, event string) (*mcrypto.PrivateSpendKey, error) {
	if event != "Refunded" && event != "Claimed" {
		return nil, errors.New("invalid event name, must be one of Claimed or Refunded")
	}

	abi, err := abi.JSON(strings.NewReader(SwapABI))
	if err != nil {
		return nil, err
	}

	data := log.Data
	res, err := abi.Unpack(event, data)
	if err != nil {
		return nil, err
	}

	if len(res) == 0 {
		return nil, errors.New("log had no parameters")
	}

	s := res[0].([32]byte)

	sk, err := mcrypto.NewPrivateSpendKey(common.Reverse(s[:]))
	if err != nil {
		return nil, err
	}

	return sk, nil
}

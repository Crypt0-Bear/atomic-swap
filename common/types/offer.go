package types

import (
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/noot/atomic-swap/common"

	"golang.org/x/crypto/sha3"
)

// Hash represents a 32-byte hash
type Hash [32]byte

// String returns the hex-encoded hash
func (h Hash) String() string {
	return hex.EncodeToString(h[:])
}

// HexToHash decodes a hex-encoded string into a hash
func HexToHash(s string) (Hash, error) {
	h, err := hex.DecodeString(s)
	if err != nil {
		return [32]byte{}, err
	}

	var hash [32]byte
	copy(hash[:], h)
	return hash, nil
}

// Offer represents a swap offer
type Offer struct {
	ID            Hash
	Provides      common.ProvidesCoin
	MinimumAmount float64
	MaximumAmount float64
	ExchangeRate  common.ExchangeRate
}

// GetID returns the ID of the offer
func (o *Offer) GetID() Hash {
	if o.ID != [32]byte{} {
		return o.ID
	}

	b, err := json.Marshal(o)
	if err != nil {
		panic(err)
	}

	o.ID = sha3.Sum256(b)
	// TODO: add some randomness in here
	return o.ID
}

// String ...
func (o *Offer) String() string {
	return fmt.Sprintf("Offer ID=%s Provides=%v MinimumAmount=%v MaximumAmount=%v ExchangeRate=%v",
		o.ID,
		o.Provides,
		o.MinimumAmount,
		o.MaximumAmount,
		o.ExchangeRate,
	)
}

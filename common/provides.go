package common

import "errors"

// ProvidesCoin represents a coin that a swap participant can provide.
type ProvidesCoin string

var (
	ProvidesXMR ProvidesCoin = "XMR" //nolint
	ProvidesETH ProvidesCoin = "ETH" //nolint
)

// NewProvidesCoin converts a string to a ProvidesCoin.
func NewProvidesCoin(s string) (ProvidesCoin, error) {
	switch s {
	case "XMR":
		return ProvidesXMR, nil
	case "ETH":
		return ProvidesETH, nil
	default:
		return "", errors.New("invalid ProvidesCoin")
	}
}

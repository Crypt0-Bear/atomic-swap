package client

import (
	"encoding/json"
	"fmt"

	"github.com/noot/atomic-swap/common"
	"github.com/noot/atomic-swap/common/rpcclient"
	"github.com/noot/atomic-swap/rpc"
)

// MakeOffer calls net_makeOffer.
func (c *Client) MakeOffer(min, max, exchangeRate float64) (string, error) {
	const (
		method = "net_makeOffer"
	)

	req := &rpc.MakeOfferRequest{
		MinimumAmount: min,
		MaximumAmount: max,
		ExchangeRate:  common.ExchangeRate(exchangeRate),
	}

	params, err := json.Marshal(req)
	if err != nil {
		return "", err
	}

	resp, err := rpcclient.PostRPC(c.endpoint, method, string(params))
	if err != nil {
		return "", err
	}

	if resp.Error != nil {
		return "", fmt.Errorf("failed to call %s: %w", method, resp.Error)
	}

	var res *rpc.MakeOfferResponse
	if err = json.Unmarshal(resp.Result, &res); err != nil {
		return "", err
	}

	return res.ID, nil
}

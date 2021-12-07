package client

import (
	"encoding/json"
	"fmt"

	"github.com/noot/atomic-swap/common"
	"github.com/noot/atomic-swap/rpc"
	"github.com/noot/atomic-swap/rpcclient"
)

func (c *Client) Initiate(maddr string, provides common.ProvidesCoin, providesAmount, desiredAmount float64) (bool, error) {
	const (
		method = "net_initiate"
	)

	req := &rpc.InitiateRequest{
		Multiaddr:      maddr,
		ProvidesCoin:   provides,
		ProvidesAmount: providesAmount,
		DesiredAmount:  desiredAmount,
	}

	params, err := json.Marshal(req)
	if err != nil {
		return false, err
	}

	resp, err := rpcclient.PostRPC(c.endpoint, method, string(params))
	if err != nil {
		return false, err
	}

	if resp.Error != nil {
		return false, fmt.Errorf("failed to call net_initiate: %w", resp.Error)
	}

	var res *rpc.InitiateResponse
	if err = json.Unmarshal(resp.Result, &res); err != nil {
		return false, err
	}

	return res.Success, nil
}

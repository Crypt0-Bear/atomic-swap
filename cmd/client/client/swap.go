package client

import (
	"encoding/json"
	"fmt"

	"github.com/noot/atomic-swap/common/rpcclient"
	"github.com/noot/atomic-swap/rpc"
)

// GetPastSwapIDs calls swap_getPastIDs
func (c *Client) GetPastSwapIDs() ([]uint64, error) {
	const (
		method = "swap_getPastIDs"
	)

	resp, err := rpcclient.PostRPC(c.endpoint, method, "{}")
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("failed to call %s: %w", method, resp.Error)
	}

	var res *rpc.GetPastIDsResponse
	if err = json.Unmarshal(resp.Result, &res); err != nil {
		return nil, err
	}

	return res.IDs, nil
}

// GetOngoingSwap calls swap_getOngoing
func (c *Client) GetOngoingSwap() (*rpc.GetOngoingResponse, error) {
	const (
		method = "swap_getOngoing"
	)

	resp, err := rpcclient.PostRPC(c.endpoint, method, "{}")
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("failed to call %s: %w", method, resp.Error)
	}

	var res *rpc.GetOngoingResponse
	if err = json.Unmarshal(resp.Result, &res); err != nil {
		return nil, err
	}

	return res, nil
}

// GetPastSwap calls swap_getPast
func (c *Client) GetPastSwap(id uint64) (*rpc.GetPastResponse, error) {
	const (
		method = "swap_getPast"
	)

	req := &rpc.GetPastRequest{
		ID: id,
	}

	params, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	resp, err := rpcclient.PostRPC(c.endpoint, method, string(params))
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, resp.Error
	}

	var res *rpc.GetPastResponse
	if err = json.Unmarshal(resp.Result, &res); err != nil {
		return nil, err
	}

	return res, nil
}

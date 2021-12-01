package config

import (
	"context"
	"fmt"
	"time"

	rpchttp "github.com/tendermint/tendermint/rpc/client/http"
)

func UpdateTrust(cfg *Config) error {
	c, err := rpchttp.New(cfg.Katzenmint.PrimaryAddress, "/websocket")
	if err != nil {
		return err
	}
	info, err := c.ABCIInfo(context.Background())
	if err != nil {
		return err
	}
	genesis, err := c.Genesis(context.Background())
	if err != nil {
		return err
	}
	blockHeight := info.Response.LastBlockHeight
	block, err := c.Block(context.Background(), &blockHeight)
	if err != nil {
		return err
	}
	if block == nil {
		return fmt.Errorf("couldn't find block: %d", blockHeight)
	}
	if genesis.Genesis.ChainID != cfg.Katzenmint.ChainID {
		return fmt.Errorf("wrong chain ID")
	}
	cfg.Katzenmint.TrustOptions.Period = 10 * time.Minute
	cfg.Katzenmint.TrustOptions.Height = blockHeight
	cfg.Katzenmint.TrustOptions.Hash = block.BlockID.Hash
	return nil
}

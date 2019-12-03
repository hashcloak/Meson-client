package main

import (
	"flag"
	"fmt"

	"github.com/hashcloak/Meson/common"
	"github.com/hashcloak/Meson-client/pkg/client"
	"github.com/katzenpost/client/config"
)

func main() {
	cfgFile := flag.String("c", "katzenpost.toml", "Path to the server config file")
	ticker := flag.String("t", "", "Ticker")
	chainID := flag.Int("chain", 1, "Chain ID for specific ETH-based chain")
	service := flag.String("s", "", "Service Name")
	rawTransactionBlob := flag.String("rt", "", "Raw Transaction blob to send over the network")
	flag.Parse()

	if *rawTransactionBlob == "" {
		panic("must specify a transaction blob in hex")
	}

	cfg, err := config.LoadFile(*cfgFile)
	if err != nil {
		panic(err)
	}

	c := client.New(cfg, *service)
	c.Start()
	reply, err := c.SendRawTransaction(rawTransactionBlob, chainID, ticker)

	fmt.Sprintf("Reply from the provider: %s", reply)
	c.Stop()

}

package main

import (
	"errors"
	"flag"
	"fmt"

	"github.com/hashcloak/Meson-client/pkg/gentxn"
	"github.com/hashcloak/Meson/plugin/pkg/common"
	"github.com/katzenpost/client"
	"github.com/katzenpost/client/config"
)

func main() {
	cfgFile := flag.String("c", "katzenpost.toml", "Path to the server config file")
	ticker := flag.String("t", "", "Ticker")
	chainID := flag.Int("chain", 1, "Chain ID for specific ETH-based chain")
	service := flag.String("s", "", "Service Name")
	rawTransactionBlob := flag.String("rt", "", "Raw Transaction blob to send over the network")
	privKey := flag.String("pk", "", "Private key used to sign the txn")
	rpcEndpoint := flag.String("rpc", "http://172.28.1.10:9545", "Ethereum rpc endpoint")
	flag.Parse()

	cfg, err := config.LoadFile(*cfgFile)
	if err != nil {
		panic(err)
	}

	cfg, linkKey := client.AutoRegisterRandomClient(cfg)
	c, err := client.New(cfg)
	if err != nil {
		panic(err)
	}

	session, err := c.NewSession(linkKey)
	if err != nil {
		panic(err)
	}

	if *rawTransactionBlob == "" {
		if *privKey == "" {
			panic("must specify a transaction blob in hex or a private key to sign a txn")
		}
		rawTransactionBlob, err = produceSignedRawTxn(privKey, rpcEndpoint, chainID)
		if err != nil {
			panic(err)
		}
	}

	// serialize our transaction inside a eth kaetzpost request message
	req := common.NewRequest(*ticker, *rawTransactionBlob, *chainID)
	mesonRequest := req.ToJson()

	mesonService, err := session.GetService(*service)
	if err != nil {
		panic(err)
	}

	reply, err := session.BlockingSendUnreliableMessage(mesonService.Name, mesonService.Provider, mesonRequest)
	if err != nil {
		panic(err)
	}
	fmt.Printf("reply: %s\n", reply)
	fmt.Println("Done. Shutting down.")
	c.Shutdown()
}

func produceSignedRawTxn(pk *string, rpcEndpoint *string, chainID *int) (*string, error) {
	ethers, err := gentxn.SetURLAndChainID(*rpcEndpoint)
	if err != nil {
		return nil, err
	}

	if ethers.ChainID.Int64() != int64(*chainID) {
		return nil, errors.New("ChainIDs are not the same between rpcEndpoint and chainID flag")
	}
	rawTxn, err := ethers.GenerateSignedRawTxn(*pk)
	return rawTxn, nil
}

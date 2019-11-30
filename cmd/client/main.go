package main

import (
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
	flag.Parse()

	if *rawTransactionBlob == "" {
		if *privKey == "" {
			panic("must specify a transaction blob in hex or a private key to sign a txn")
		}
	}

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

	raw := *rawTransactionBlob
	if *privKey != "" {
		raw = gentxn.GenerateRawTxn(privKey, "http://localhost:19545")
	}

	// serialize our transaction inside a eth kaetzpost request message
	req := common.NewRequest(*ticker, raw, *chainID)
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

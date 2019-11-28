package main

import (
	"encoding/hex"
	"flag"
	"fmt"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/hashcloak/Meson/common"
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

	key, err := crypto.HexToECDSA(*privKey)
	if err != nil {
		panic(err)
	}

	pubKey := crypto.PubkeyToAddress(key.PublicKey)
	fmt.Printf("Using address: \n0x%v\n", hex.EncodeToString(pubKey.Bytes()))

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

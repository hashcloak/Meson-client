package main

import (
    "fmt"
    "flag"
    "math/big"

    "github.com/hashcloak/Meson-client"
    "github.com/ethereum/go-ethereum/common"
    "github.com/ethereum/go-ethereum/core/types"
    "github.com/ethereum/go-ethereum/crypto"
    "github.com/katzenpost/client"
	"github.com/katzenpost/client/config"
	"github.com/katzenpost/currency/common"
	"github.com/ugorji/go/codec"
)

type Transaction struct {
    Nonce uint64
    ToAddress string
    Amount *big.Int
    GasLimit uint64
    GasPrice *big.Int
    Data []byte
}

func createRawTransactionBlob(transaction *Transaction, chainId *big.Int, privKey *ecdsa.PrivateKey) (string, error){
    tx := types.NewTransaction(nonce, common.HexToAddress(address), amount, gasLimit, gasPrice, data)
    signedTx := types.SignTx(tx, types.NewEIP155Signer(chainId), privKey)
    ts := types.Transactions{signedTx}
    return fmt.Sprintf("%x", ts.getRlp(0)), nil
}

func main() {
    toAddr := flag.String()
    amount := flag.Int()
    gasLimit := flag.Int()
    gasPrice := flag.Int()

    cfgFile := flag.Strin()
    ticker := flag.String()
    service := flag.String()
    flag.parse()

    // TODO: Error check all the passed in commandline options

    // TODO: Get Private Key from keystore

    // TODO: Create raw transaction blob

    cfg, linkKey := client.AutoRegisterRandomClient(cfg)
	c, err := client.New(cfg)
	if err != nil {
		panic(err)
	}

	session, err := c.NewSession(linkKey)
	if err != nil {
		panic(err)
	}

	// serialize our transaction inside a zcash kaetzpost request message
	req := common.NewRequest(*ticker, *hexBlob)
	currencyRequest := req.ToJson()

	currencyService, err := session.GetService(*service)
	if err != nil {
		panic(err)
	}

	reply, err := session.BlockingSendUnreliableMessage(currencyService.Name, currencyService.Provider, currencyRequest)
	if err != nil {
		panic(err)
	}
	fmt.Printf("reply: %s\n", reply)
	fmt.Println("Done. Shutting down.")
	c.Shutdown()
}

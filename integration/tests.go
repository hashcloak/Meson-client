package main

import (
	"context"
	"encoding/hex"
	"flag"
	"fmt"
	"math/big"

	ethCommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	ethTypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/hashcloak/Meson-plugin/pkg/common"
	"github.com/katzenpost/client"
	"github.com/katzenpost/client/config"
)

type TestSuit struct {
	pk                *string
	rpcURL            *string
	ticker            *string
	signedTransaction *string
	transactionHash   *[]byte
}

func getRPCUrl(currencyTomlPath string) *string {
	val := "https://goerli.hashcloak.com"
	return &val
}

func main() {
	cfgFile := flag.String("c", "client.toml", "Path to the server config file")
	ticker := flag.String("t", "", "Ticker")
	service := flag.String("s", "", "Service Name")
	privKey := flag.String("pk", "", "Private key used to sign the txn")
	flag.Parse()

	cfg, err := config.LoadFile(*cfgFile)
	if err != nil {
		panic(err)
	}
	if *privKey == "" {
		panic("must specify a transaction blob in hex or a private key to sign a txn")
	}

	testSuit := &TestSuit{
		pk:     privKey,
		ticker: ticker,
		rpcURL: getRPCUrl(""),
	}

	if err := testSuit.produceSignedRawTxn(); err != nil {
		panic("Raw txn error: " + err.Error())
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
	mesonService, err := session.GetService(*service)
	if err != nil {
		panic("Client error" + err.Error())
	}

	mesonRequest := common.NewRequest(*ticker, *testSuit.signedTransaction).ToJson()
	reply, err := session.BlockingSendUnreliableMessage(mesonService.Name, mesonService.Provider, mesonRequest)
	if err != nil {
		panic("Meson Request Error" + err.Error())
	}
	fmt.Printf("reply: %s\n", reply)
	fmt.Println("Done. Shutting down.")
	c.Shutdown()
}

func (s *TestSuit) produceSignedRawTxn() error {
	var err error
	switch *s.ticker {
	case "tbnb":
		err = s.signEthereumRawTxn()
	case "gor":
		fmt.Println("HEYO")
		err = s.signEthereumRawTxn()
	}
	return err
}

// signRawRawTransaction just signs a txn with
func (s *TestSuit) signEthereumRawTxn() error {
	ethclient, err := ethclient.Dial(*s.rpcURL)
	key, err := crypto.HexToECDSA(*s.pk)
	if err != nil {
		return err
	}

	nonce, err := ethclient.PendingNonceAt(
		context.Background(),
		crypto.PubkeyToAddress(key.PublicKey),
	)

	if err != nil {
		return err
	}

	gasPrice, err := ethclient.SuggestGasPrice(context.Background())
	if err != nil {
		return err
	}

	to := ethCommon.HexToAddress(crypto.PubkeyToAddress(key.PublicKey).Hex())
	tx := ethTypes.NewTransaction(
		nonce,
		to,
		big.NewInt(123),
		uint64(21000),
		gasPrice,
		[]byte(""),
	)
	tempValue := tx.Hash().Bytes()
	s.transactionHash = &tempValue

	id, err := ethclient.ChainID(context.Background())
	if err != nil {
		return err
	}
	signedTx, err := ethTypes.SignTx(tx, types.NewEIP155Signer(id), key)
	if err != nil {
		return err
	}

	tempString := "0x" + hex.EncodeToString(types.Transactions{signedTx}.GetRlp(0))
	s.signedTransaction = &tempString
	return nil
}

func (s *TestSuit) checkTransactionIsAccepted() error {
	switch *s.ticker {
	case "tbnb":
		return s.signEthereumRawTxn()
	case "gor":
		return s.checkEthereumTransacton()

	default:
		return fmt.Errorf("Wrong ticker")
	}
}

func (s *TestSuit) checkEthereumTransacton() error {
	ethclient, err := ethclient.Dial(*s.rpcURL)
	if err != nil {
		return err
	}
	hash := ethCommon.BytesToHash(*s.transactionHash)
	receipt, err := ethclient.TransactionReceipt(context.Background(), hash)
	if err != nil {
		return err
	}

	fmt.Println(receipt)

	return nil
}

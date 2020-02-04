package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"math/big"
	"os"

	ethCommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	ethTypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/hashcloak/Meson-plugin/pkg/common"
	"github.com/katzenpost/client"
	"github.com/katzenpost/client/config"
	"github.com/katzenpost/core/crypto/ecdh"
)

type TestSuit struct {
	pk                *string
	rpcURL            *string
	ticker            *string
	signedTransaction *string
	transactionHash   *[]byte
}

type MesonReply struct {
	Message    string `json:"Message,omitempty"`
	StatusCode uint   `json:"StatusCode"`
	Version    uint   `json:"Version"`
}

func getRPCUrl(currencyTomlPath string) *string {
	val := "https://goerli.hashcloak.com"
	return &val
}

func setupMesonClient(cfg *config.Config) (*config.Config, *ecdh.PrivateKey) {
	maxTries := 10
	defer func() {
		if r := recover(); r != nil {
			if r == "pki: requested epoch will never get a document" {
				maxTries++
				fmt.Println("Recovered in f", r)
			}
		}
	}()
	return client.AutoRegisterRandomClient(cfg)
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

	cfg, linkKey := setupMesonClient(cfg)
	//cfg, linkKey := client.AutoRegisterRandomClient(cfg)

	c, err := client.New(cfg)
	if err != nil {
		panic("New Client error: " + err.Error())
	}
	session, err := c.NewSession(linkKey)
	if err != nil {
		panic("Session error: " + err.Error())
	}

	// serialize our transaction inside a eth kaetzpost request message
	mesonService, err := session.GetService(*service)
	if err != nil {
		panic("Client error: " + err.Error())
	}

	mesonRequest := common.NewRequest(*ticker, *testSuit.signedTransaction).ToJson()
	reply, err := session.BlockingSendUnreliableMessage(mesonService.Name, mesonService.Provider, mesonRequest)
	if err != nil {
		panic("Meson Request Error" + err.Error())
	}
	reply = bytes.TrimRight(reply, "\x00")
	fmt.Printf("reply: %s\n", reply)
	var mesonReply MesonReply
	err = json.Unmarshal(reply, &mesonReply)
	if err != nil {
		panic("Unmarshal error: " + err.Error())
	}
	testSuit.checkTransactionIsAccepted()
	if mesonReply.Message == "success" {
		fmt.Println("Messages are the same")
	} else {
		os.Exit(-1)
	}
	fmt.Println("Done. Shutting down.")
	c.Shutdown()
}

func (s *TestSuit) produceSignedRawTxn() error {
	var err error
	switch *s.ticker {
	case "tbnb":
		err = s.signEthereumRawTxn()
	case "gor":
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
		return s.checkEthereumTransaction()
	case "gor":
		return s.checkEthereumTransaction()

	default:
		return fmt.Errorf("Wrong ticker")
	}
}

func (s *TestSuit) checkEthereumTransaction() error {
	fmt.Println("Checking for transaction...")
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

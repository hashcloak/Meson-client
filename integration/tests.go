package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"math/big"

	ethCommon "github.com/ethereum/go-ethereum/common"
	ethTypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"

	client "github.com/hashcloak/Meson-client"
	"github.com/hashcloak/Meson-client/config"
	currencyConfig "github.com/hashcloak/Meson-plugin/pkg/config"
)

type TestSuite struct {
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

func (s *TestSuite) produceSignedRawTxn() error {
	var err error
	switch *s.ticker {
	case "tbnb":
		err = s.signCosmosRawTxn()
	case "gor":
		err = s.signEthereumRawTxn()
	}
	return err
}
func (s *TestSuite) signCosmosRawTxn() error {
	return nil
}

// signRawRawTransaction just signs a txn with
func (s *TestSuite) signEthereumRawTxn() error {
	ethclient, err := ethclient.Dial(*s.rpcURL)
	if err != nil {
		return err
	}
	key, err := crypto.HexToECDSA(*s.pk)
	if err != nil {
		return err
	}
	address := crypto.PubkeyToAddress(key.PublicKey)
	nonce, err := ethclient.PendingNonceAt(context.Background(), address)
	if err != nil {
		return err
	}
	gasPrice, err := ethclient.SuggestGasPrice(context.Background())
	if err != nil {
		return err
	}
	tx := ethTypes.NewTransaction(
		nonce,
		address,
		big.NewInt(123),
		uint64(21000),
		gasPrice,
		[]byte(""),
	)

	id, err := ethclient.ChainID(context.Background())
	if err != nil {
		return err
	}
	signedTx, err := ethTypes.SignTx(tx, ethTypes.NewEIP155Signer(id), key)
	if err != nil {
		return err
	}

	*s.transactionHash = signedTx.Hash().Bytes()
	*s.signedTransaction = "0x" + hex.EncodeToString(ethTypes.Transactions{signedTx}.GetRlp(0))
	return nil
}

func (s *TestSuite) checkTransactionIsAccepted() error {
	fmt.Println("Checking for transaction...")
	switch *s.ticker {
	case "tbnb":
		return s.checkCosmosTransaction()
	case "gor":
		return s.checkEthereumTransaction()
	default:
		return fmt.Errorf("Wrong ticker")
	}
}

func (s *TestSuite) checkCosmosTransaction() error {
	return nil
}

func (s *TestSuite) checkEthereumTransaction() error {
	ethclient, err := ethclient.Dial(*s.rpcURL)
	if err != nil {
		return err
	}

	hash := ethCommon.BytesToHash(*s.transactionHash)
	_, pending, err := ethclient.TransactionByHash(context.Background(), hash)
	if err != nil {
		return err
	}

	if !pending {
		receipt, err := ethclient.TransactionReceipt(context.Background(), hash)
		if err != nil {
			return err
		}
		if receipt.BlockNumber == nil {
			return fmt.Errorf("Transaction is not pending and has no blocknumber")
		}
	}
	fmt.Printf("Ethereum transaction found: %v\n", hash.String())
	return nil
}

func main() {
	clientToml := flag.String("c", "client.toml", "Path to the server config file")
	privKey := flag.String("pk", "", "Private key used to sign the txn")
	currencyConfigPath := flag.String("k", "", "The currency.toml path")
	flag.Parse()

	if *privKey == "" {
		panic("Private key not provided")
	}
	if *currencyConfigPath == "" {
		panic("You need to specify the currency.toml file used by the Meson plugin")
	}
	cfg, err := currencyConfig.LoadFile(*currencyConfigPath)
	if err != nil {
		panic("Currency config read error: " + err.Error())
	}

	testSuite := &TestSuite{
		pk:                privKey,
		ticker:            &cfg.Ticker,
		rpcURL:            &cfg.RPCURL,
		signedTransaction: new(string),
		transactionHash:   new([]byte),
	}

	if err := testSuite.produceSignedRawTxn(); err != nil {
		panic("ERROR Signing raw transaction: " + err.Error())
	}

	clientCfg, err := config.LoadFile(*clientToml)
	if err != nil {
		panic("ERROR In loading client config: " + err.Error())
	}
	client, err := client.New(clientCfg, *testSuite.ticker)
	if err != nil {
		panic("ERROR In creating new client: " + err.Error())
	}
	_ = client.Start()
	reply, err := client.SendRawTransaction(testSuite.signedTransaction, testSuite.ticker)
	if err != nil {
		panic("ERROR Send raw transaction: " + err.Error())
	}

	reply = bytes.TrimRight(reply, "\x00")
	var mesonReply MesonReply
	if err := json.Unmarshal(reply, &mesonReply); err != nil {
		panic("ERROR Unmarshal: " + err.Error())
	}
	if mesonReply.Message != "success" {
		panic("ERROR Message was not successful, message: " + mesonReply.Message)
	}
	fmt.Println("Transaction submitted. Shutting down meson client")
	client.Shutdown()

	if err := testSuite.checkTransactionIsAccepted(); err != nil {
		panic("Transaction error: " + err.Error())
	}
}

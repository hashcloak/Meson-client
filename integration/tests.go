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
	"github.com/ethereum/go-ethereum/core/types"
	ethTypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/hashcloak/Meson-plugin/pkg/common"
	currencyConfig "github.com/hashcloak/Meson-plugin/pkg/config"
	"github.com/katzenpost/client"
	"github.com/katzenpost/client/config"
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

func getCurrencyRPCUrl(currencyTomlPath *string) (*string, error) {
	cfg, err := currencyConfig.LoadFile(*currencyTomlPath)
	if err != nil {
		return nil, err
	}
	return &cfg.RPCURL, nil
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

	id, err := ethclient.ChainID(context.Background())
	if err != nil {
		return err
	}
	signedTx, err := ethTypes.SignTx(tx, types.NewEIP155Signer(id), key)
	if err != nil {
		return err
	}

	*s.transactionHash = signedTx.Hash().Bytes()
	*s.signedTransaction = "0x" + hex.EncodeToString(types.Transactions{signedTx}.GetRlp(0))
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

func (s *TestSuite) mesonRequest(cfgFile *string) error {
	cfg, err := config.LoadFile(*cfgFile)
	if err != nil {
		panic("Config file error: " + err.Error())
	}
	cfg, linkKey := client.AutoRegisterRandomClient(cfg)
	c, err := client.New(cfg)
	if err != nil {
		panic("New Client error: " + err.Error())
	}

	session, err := c.NewSession(linkKey)
	if err != nil {
		panic("Session error: " + err.Error())
	}

	mesonService, err := session.GetService(*s.ticker)
	if err != nil {
		panic("Client error: " + err.Error())
	}

	mesonRequest := common.NewRequest(*s.ticker, *s.signedTransaction).ToJson()
	reply, err := session.BlockingSendUnreliableMessage(mesonService.Name, mesonService.Provider, mesonRequest)
	if err != nil {
		panic("Meson Request Error " + err.Error())
	}
	reply = bytes.TrimRight(reply, "\x00")
	var mesonReply MesonReply
	if err := json.Unmarshal(reply, &mesonReply); err != nil {
		panic("Unmarshal error: " + err.Error())
	}
	if mesonReply.Message != "success" {
		panic("Message was not a success: " + mesonReply.Message)
	}
	fmt.Println("Transaction submitted. Shutting down meson client")
	c.Shutdown()
	return nil
}

func main() {
	cfgFile := flag.String("c", "client.toml", "Path to the server config file")
	ticker := flag.String("t", "", "Ticker")
	privKey := flag.String("pk", "", "Private key used to sign the txn")
	currencyConfigPath := flag.String("k", "", "The currency.toml path")
	flag.Parse()

	if *privKey == "" {
		panic("Or a private key to sign a txn")
	}
	if *currencyConfigPath == "" {
		panic("You need to specify the currency.toml file used by the Meson plugin")
	}
	rpcURL, err := getCurrencyRPCUrl(currencyConfigPath)
	if err != nil {
		panic("Currency config read error: " + err.Error())
	}

	testSuite := &TestSuite{
		pk:                privKey,
		ticker:            ticker,
		rpcURL:            rpcURL,
		signedTransaction: new(string),
		transactionHash:   new([]byte),
	}

	if err := testSuite.produceSignedRawTxn(); err != nil {
		panic("Raw txn error: " + err.Error())
	}

	if err := testSuite.mesonRequest(cfgFile); err != nil {
		panic(err)
	}

	if err := testSuite.checkTransactionIsAccepted(); err != nil {
		panic("Transaction error: " + err.Error())
	}
}

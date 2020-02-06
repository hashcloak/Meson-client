package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"math/big"
	"net/http"
	"os"
	"strings"

	sdk "github.com/binance-chain/go-sdk/client"
	bnbTypes "github.com/binance-chain/go-sdk/common/types"
	"github.com/binance-chain/go-sdk/keys"
	"github.com/binance-chain/go-sdk/types/msg"
	bnbTx "github.com/binance-chain/go-sdk/types/tx"
	ethCommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	ethTypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	tendermintCrypto "github.com/tendermint/tendermint/crypto"

	"github.com/hashcloak/Meson-plugin/pkg/common"
	currencyConfig "github.com/hashcloak/Meson-plugin/pkg/config"
	"github.com/katzenpost/client"
	"github.com/katzenpost/client/config"
	"github.com/katzenpost/core/crypto/ecdh"
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

func getCosmosAccountInfo(key keys.KeyManager) (*bnbTypes.BalanceAccount, error) {
	clientSDK, err := sdk.NewDexClient("testnet-dex.binance.org", bnbTypes.TestNetwork, key)
	if err != nil {
		return nil, err
	}
	return clientSDK.GetAccount(key.GetAddr().String())
}

func getCurrencyRPCUrl(currencyTomlPath *string) (*string, error) {
	cfg, err := currencyConfig.LoadFile(*currencyTomlPath)
	if err != nil {
		return nil, err
	}
	return &cfg.RPCURL, nil
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
	key, err := keys.NewPrivateKeyManager(*s.pk)
	if err != nil {
		return err
	}
	mess := []msg.Msg{
		msg.CreateSendMsg(
			key.GetAddr(),
			bnbTypes.Coins{
				bnbTypes.Coin{Denom: "BNB", Amount: 1},
			},
			[]msg.Transfer{
				{key.GetAddr(), bnbTypes.Coins{bnbTypes.Coin{Denom: "BNB", Amount: 1}}},
			},
		),
	}
	account, err := getCosmosAccountInfo(key)
	if err != nil {
		return err
	}
	m := bnbTx.StdSignMsg{
		Msgs:          mess,
		Source:        0,
		Sequence:      account.Sequence,
		AccountNumber: account.Number,
		ChainID:       "Binance-Chain-Nile",
	}
	signed, err := key.Sign(m)
	*s.signedTransaction = hex.EncodeToString(signed)
	*s.transactionHash = tendermintCrypto.Sha256(signed)
	return err
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
	txnUpperCase := strings.ToUpper(hex.EncodeToString(*s.transactionHash))
	fmt.Printf("Checking for transaction: %v\n", txnUpperCase)

	//https://tbinance.hashcloak.com/tx?hash=0x3A6D8270FC9C579282C07CAFD15E80086851B383208880EAAA09A6F6BB708E5D&prove=true
	url := "https://tbinance.hashcloak.com/tx?hash=0x%s&prove=%s"
	url = fmt.Sprintf(url, txnUpperCase, "false")
	fmt.Println(url)

	httpResponse, err := http.Post(url, "application/json", nil)
	if err != nil {
		panic(err)
	}
	defer httpResponse.Body.Close()
	bodyBytes, err := ioutil.ReadAll(httpResponse.Body)
	if err != nil {
		panic(err)
	}

	fmt.Printf("Response: %+v\n", string(bodyBytes))
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
	cfgFile := flag.String("c", "client.toml", "Path to the server config file")
	ticker := flag.String("t", "", "Ticker")
	service := flag.String("s", "", "Service Name")
	privKey := flag.String("pk", "", "Private key used to sign the txn")
	currencyConfigPath := flag.String("k", "", "The currency.toml path")
	flag.Parse()

	cfg, err := config.LoadFile(*cfgFile)
	if err != nil {
		panic("Config file error: " + err.Error())
	}
	if *privKey == "" {
		panic("must specify a transaction blob in hex or a private key to sign a txn")
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

	cfg, linkKey := setupMesonClient(cfg)
	// Alternative method that doesn't catch errors
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

	mesonRequest := common.NewRequest(*ticker, *testSuite.signedTransaction).ToJson()
	reply, err := session.BlockingSendUnreliableMessage(mesonService.Name, mesonService.Provider, mesonRequest)
	if err != nil {
		panic("Meson Request Error " + err.Error())
	}
	reply = bytes.TrimRight(reply, "\x00")
	var mesonReply MesonReply
	err = json.Unmarshal(reply, &mesonReply)
	if err != nil {
		panic("Unmarshal error: " + err.Error())
	}
	if mesonReply.Message != "success" {
		fmt.Println("Message was not a success: ", mesonReply.Message)
		os.Exit(-1)
	}
	fmt.Println("Transaction submitted. Shutting down meson client")
	c.Shutdown()

	if err := testSuite.checkTransactionIsAccepted(); err != nil {
		panic("Transaction error: " + err.Error())
	}
}

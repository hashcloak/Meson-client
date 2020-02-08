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
	"strings"

	sdk "github.com/binance-chain/go-sdk/client"
	bnbTypes "github.com/binance-chain/go-sdk/common/types"
	"github.com/binance-chain/go-sdk/keys"
	"github.com/binance-chain/go-sdk/types/msg"
	bnbTx "github.com/binance-chain/go-sdk/types/tx"
	tendermintCrypto "github.com/tendermint/tendermint/crypto"

	ethCommon "github.com/ethereum/go-ethereum/common"
	ethTypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"

	client "github.com/hashcloak/Meson-client"
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
	txnUpperCase := strings.ToUpper(hex.EncodeToString(*s.transactionHash))
	url := "https://tbinance.hashcloak.com/tx?hash=0x%s&prove=%s"
	url = fmt.Sprintf(url, txnUpperCase, "false")
	httpResponse, err := http.Post(url, "application/json", nil)
	if err != nil {
		return err
	}
	defer httpResponse.Body.Close()
	bodyBytes, err := ioutil.ReadAll(httpResponse.Body)
	if err != nil {
		return err
	}
	type result struct {
		Hash string `json:"hash"`
	}
	type cosmosReply struct {
		Result result `json:"result"`
	}
	var reply cosmosReply
	if err := json.Unmarshal(bodyBytes, &reply); err != nil {
		return err
	}
	if txnUpperCase != reply.Result.Hash {
		return fmt.Errorf("Error. Got: %v, wanted: %v", txnUpperCase, reply.Result.Hash)
	}
	fmt.Println("Found cosmos txn: ", reply.Result.Hash)
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
	ticker := flag.String("t", "", "Ticker")
	service := flag.String("s", "", "service")
	privKey := flag.String("pk", "", "Private key used to sign the txn")
	currencyConfigPath := flag.String("k", "", "The currency.toml path")
	flag.Parse()

	if *service != *ticker {
		panic("-s service and -t ticker are not the same")
	}
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
		panic("ERROR Signing raw transaction: " + err.Error())
	}

	client, err := client.New(*clientToml, *testSuite.ticker)
	if err != nil {
		panic("ERROR In creating new client: " + err.Error())
	}
	client.Start()
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

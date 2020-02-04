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

func getCosmosAccountInfo(key keys.KeyManager) (*bnbTypes.BalanceAccount, error) {
	clientSDK, err := sdk.NewDexClient("testnet-dex.binance.org", bnbTypes.TestNetwork, key)
	if err != nil {
		return nil, err
	}
	return clientSDK.GetAccount(key.GetAddr().String())
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
		pk:                privKey,
		ticker:            ticker,
		rpcURL:            getRPCUrl(""),
		signedTransaction: new(string),
		transactionHash:   new([]byte),
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
	if err := testSuit.checkTransactionIsAccepted(); err != nil {
		panic("Transaction error: " + err.Error())
	}
	fmt.Println("Done. Shutting down.")
	c.Shutdown()
}

func (s *TestSuit) produceSignedRawTxn() error {
	var err error
	switch *s.ticker {
	case "tbnb":
		err = s.signCosmosRawTxn()
	case "gor":
		err = s.signEthereumRawTxn()
	}
	return err
}
func (s *TestSuit) signCosmosRawTxn() error {
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

func (s *TestSuit) checkTransactionIsAccepted() error {
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

func (s *TestSuit) checkCosmosTransaction() error {
	key, err := keys.NewPrivateKeyManager(*s.pk)
	clientSDK, err := sdk.NewDexClient("testnet-dex.binance.org", bnbTypes.TestNetwork, key)
	if err != nil {
		return err
	}
	result, err := clientSDK.GetTx(hex.EncodeToString(*s.transactionHash))
	if err != nil {
		return err
	}
	fmt.Println("RESULT: ", result)
	return nil
}

func (s *TestSuit) checkEthereumTransaction() error {
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
	return nil
}

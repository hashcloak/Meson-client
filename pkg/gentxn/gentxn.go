package gentxn

import (
	"context"
	"encoding/hex"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"log"
	"math/big"
)

// GenerateRawTxn just signs a txn with
func GenerateRawTxn(pk *string, rpcEndpoint string) string {

	key, err := crypto.HexToECDSA(*pk)
	if err != nil {
		panic(err)
	}

	ethclient, err := ethclient.Dial(rpcEndpoint)
	// Note that this is not using the mixnet to obtain the nonce value.
	// This is just to facilitate testing the mixnet.
	nonce, err := ethclient.PendingNonceAt(context.Background(), crypto.PubkeyToAddress(key.PublicKey))
	if err != nil {
		log.Fatal(err)
	}

	value := big.NewInt(123)
	gasLimit := uint64(21000)
	gasPrice, err := ethclient.SuggestGasPrice(context.Background())
	if err != nil {
		log.Fatal(err)
	}

	toAddress := common.HexToAddress(crypto.PubkeyToAddress(key.PublicKey).Hex())
	var data []byte
	tx := types.NewTransaction(
		nonce,
		toAddress,
		value,
		gasLimit, gasPrice, data)

	chainID, err := ethclient.NetworkID(context.Background())
	if err != nil {
		log.Fatal(err)
	}

	signedTx, err := types.SignTx(tx, types.NewEIP155Signer(chainID), key)

	ts := types.Transactions{signedTx}
	return hex.EncodeToString(ts.GetRlp(0))
}

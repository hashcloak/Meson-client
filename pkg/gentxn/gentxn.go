package gentxn

import (
	"context"
	"encoding/hex"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"math/big"
)

// Ethers basic ethereum node interface
type Ethers struct {
	ChainID   *big.Int
	ethclient *ethclient.Client
}

// SetURLAndChainID init for ethers
func SetURLAndChainID(rpcEndpoint string) (Ethers, error) {
	ethclient, err := ethclient.Dial(rpcEndpoint)
	if err != nil {
		return Ethers{}, err
	}
	chainID, err := ethclient.ChainID(context.Background())
	return Ethers{chainID, ethclient}, err
}

func (e *Ethers) obtainNonce(pubAddress common.Address) (uint64, error) {
	nonce, err := e.ethclient.PendingNonceAt(context.Background(), pubAddress)
	if err != nil {
		return 0, err
	}
	return nonce, nil
}

func (e *Ethers) obtainGasPrice() (*big.Int, error) {
	gasPrice, err := e.ethclient.SuggestGasPrice(context.Background())
	if err != nil {
		return nil, err
	}
	return gasPrice, nil
}

// GenerateSignedRawTxn just signs a txn with
func (e *Ethers) GenerateSignedRawTxn(pk string) (*string, error) {

	key, err := crypto.HexToECDSA(pk)
	if err != nil {
		return nil, err
	}

	nonce, err := e.obtainNonce(crypto.PubkeyToAddress(key.PublicKey))
	if err != nil {
		return nil, err
	}
	gasPrice, err := e.obtainGasPrice()
	if err != nil {
		return nil, err
	}
	to := common.HexToAddress(crypto.PubkeyToAddress(key.PublicKey).Hex())
	tx := types.NewTransaction(
		nonce,
		to,
		big.NewInt(123),
		uint64(21000),
		gasPrice,
		[]byte(""),
	)

	signedTx, err := types.SignTx(tx, types.NewEIP155Signer(e.ChainID), key)
	if err != nil {
		return nil, err
	}
	txn := "0x" + hex.EncodeToString(types.Transactions{signedTx}.GetRlp(0))
	return &txn, nil
}

package minclient

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/binary"
	"fmt"
	"io/ioutil"
	stdlog "log"
	"os"
	"path/filepath"
	"testing"
	"time"

	kpki "github.com/hashcloak/katzenmint-pki"
	"github.com/hashcloak/katzenmint-pki/s11n"
	"github.com/hashcloak/katzenmint-pki/testutil"
	"github.com/katzenpost/core/crypto/rand"
	katlog "github.com/katzenpost/core/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	log "github.com/tendermint/tendermint/libs/log"
	"github.com/tendermint/tendermint/light"
	"github.com/tendermint/tendermint/light/provider/http"
	rpcclient "github.com/tendermint/tendermint/rpc/client"
	"github.com/tendermint/tendermint/rpc/client/local"
	rpctest "github.com/tendermint/tendermint/rpc/test"
	dbm "github.com/tendermint/tm-db"

	// ics23 "github.com/confio/ics23/go"
	"github.com/cosmos/iavl"
	"github.com/stretchr/testify/mock"

	abci "github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/crypto/merkle"
	tmbytes "github.com/tendermint/tendermint/libs/bytes"
	lightrpc "github.com/tendermint/tendermint/light/rpc"
	lcmock "github.com/tendermint/tendermint/light/rpc/mocks"
	tmcrypto "github.com/tendermint/tendermint/proto/tendermint/crypto"
	rpcmock "github.com/tendermint/tendermint/rpc/client/mocks"
	ctypes "github.com/tendermint/tendermint/rpc/core/types"
	"github.com/tendermint/tendermint/types"
)

var (
	testDir    string
	abciClient *local.Local
)

func newDiscardLogger() (logger log.Logger) {
	logger = log.NewTMLogger(log.NewSyncWriter(ioutil.Discard))
	return
}

func getEpoch(abciClient rpcclient.Client, require *require.Assertions) uint64 {
	appInfo, err := abciClient.ABCIInfo(context.Background())
	require.NoError(err)
	epochByt := kpki.DecodeHex(appInfo.Response.Data)
	epoch, err := binary.ReadUvarint(bytes.NewReader(epochByt))
	require.NoError(err)
	return epoch
}

// TestGetDocument tests the functionality of Meson universe
func TestGetDocument(t *testing.T) {
	var (
		assert     = assert.New(t)
		require    = require.New(t)
		config     = rpctest.GetConfig()
		chainID    = config.ChainID()
		rpcAddress = config.RPC.ListenAddress
	)

	// Give Tendermint time to generate some blocks
	time.Sleep(5 * time.Second)

	// Get an initial trusted block
	primary, err := http.New(chainID, rpcAddress)
	require.NoError(err)

	block, err := primary.LightBlock(context.Background(), 0)
	require.NoError(err)

	trustOptions := light.TrustOptions{
		Period: 10 * time.Minute,
		Height: block.Height,
		Hash:   block.Hash(),
	}

	// Setup a pki client
	logPath := filepath.Join(testDir, "pkiclient_log")
	logBackend, err := katlog.New(logPath, "INFO", true)
	require.NoError(err)

	pkiClient, err := NewPKIClient(&PKIClientConfig{
		LogBackend:         logBackend,
		ChainID:            chainID,
		TrustOptions:       trustOptions,
		PrimaryAddress:     rpcAddress,
		WitnessesAddresses: []string{rpcAddress},
		DatabaseName:       "pkiclient_db",
		DatabaseDir:        testDir,
		RpcAddress:         rpcAddress,
	})
	require.NoError(err)

	// Get the upcoming epoch
	epoch := getEpoch(abciClient, require)
	epoch += 1

	// Create a document
	_, docSer := testutil.CreateTestDocument(require, epoch)
	docTest, err := s11n.VerifyAndParseDocument(docSer)
	require.NoError(err)

	rawTx := kpki.Transaction{
		Version: kpki.ProtocolVersion,
		Epoch:   epoch,
		Command: kpki.AddConsensusDocument,
		Payload: string(docSer),
	}
	_, privKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(err)

	rawTx.AppendSignature(privKey)

	// Upload the document
	resp, err := pkiClient.PostTx(context.Background(), rawTx)
	require.NoError(err)
	require.NotNil(resp)

	// Get the document and verify
	err = rpcclient.WaitForHeight(abciClient, resp.Height+1, nil)
	require.NoError(err)

	doc, _, err := pkiClient.Get(context.Background(), epoch)
	require.NoError(err)
	assert.Equal(docTest, doc, "Got an incorrect document")

	// Try getting a non-existing document
	_, _, err = pkiClient.Get(context.Background(), epoch+1)
	assert.NotNil(err, "Got a document that should not exist")
}

// TestMain tests the katzenmint-pki
func TestMain(m *testing.M) {
	var err error

	// set up test directory
	testDir, err = ioutil.TempDir("", "pkiclient_test")
	if err != nil {
		stdlog.Fatal(err)
	}

	// start katzenmint node in the background to test against
	db := dbm.NewMemDB()
	logger := newDiscardLogger()
	app := kpki.NewKatzenmintApplication(db, logger)
	node := rpctest.StartTendermint(app, rpctest.SuppressStdout)
	abciClient = local.New(node)

	code := m.Run()

	// and shut down properly at the end
	rpctest.StopTendermint(node)
	db.Close()
	os.RemoveAll(testDir)
	os.Exit(code)
}

type testOp struct {
	Key   []byte
	Proof *iavl.RangeProof
}

func (op testOp) GetKey() []byte {
	return op.Key
}

func (op testOp) ProofOp() tmcrypto.ProofOp {
	proof := iavl.NewValueOp(op.Key, op.Proof)
	return proof.ProofOp()
}

func (op testOp) Run(args [][]byte) ([][]byte, error) {
	root := op.Proof.ComputeRootHash()
	switch len(args) {
	case 0:
		if err := op.Proof.Verify(root); err != nil {
			return nil, fmt.Errorf("root did not verified: %+v", err)
		}
	case 1:
		if err := op.Proof.VerifyAbsence(args[0]); err != nil {
			return nil, fmt.Errorf("proof did not verified: %+v", err)
		}
	default:
		return nil, fmt.Errorf("args must be length 0 or 1, got: %d", len(args))
	}

	return [][]byte{root}, nil
}

// TestMockPKIClientGetDocument tests PKI Client get document and verifies proofs.
func TestMockPKIClientGetDocument(t *testing.T) {
	var (
		require            = require.New(t)
		epoch       uint64 = 1
		blockHeight int64  = 1
	)

	// create a test document
	_, docSer := testutil.CreateTestDocument(require, epoch)
	testDoc, err := s11n.VerifyAndParseDocument(docSer)
	require.NoError(err)

	var (
		key   = []byte{1}
		value = make([]byte, len(docSer))
	)

	n := copy(value, docSer)
	require.Equal(n, len(docSer))

	// create iavl tree
	tree, err := iavl.NewMutableTree(dbm.NewMemDB(), 100)
	require.NoError(err)

	tree.Set(key, value)

	rawDoc, proof, err := tree.GetWithProof(key)
	require.NoError(err)
	require.Equal(rawDoc, docSer)

	testOp := &testOp{
		Key:   key,
		Proof: proof,
	}

	query := kpki.Query{
		Version: kpki.ProtocolVersion,
		Epoch:   epoch,
		Command: kpki.GetConsensus,
		Payload: "",
	}
	rawQuery, err := kpki.EncodeJson(query)
	require.NoError(err)

	// moke the abci query
	next := &rpcmock.Client{}
	next.On(
		"ABCIQueryWithOptions",
		context.Background(),
		mock.AnythingOfType("string"),
		tmbytes.HexBytes(rawQuery),
		mock.AnythingOfType("client.ABCIQueryOptions"),
	).Return(&ctypes.ResultABCIQuery{
		Response: abci.ResponseQuery{
			Code:   0,
			Key:    testOp.GetKey(),
			Value:  value,
			Height: blockHeight,
			ProofOps: &tmcrypto.ProofOps{
				Ops: []tmcrypto.ProofOp{testOp.ProofOp()},
			},
		},
	}, nil)

	// mock the abci info
	epochByt := make([]byte, 8)
	binary.PutUvarint(epochByt[:], epoch)
	next.On(
		"ABCIInfo",
		context.Background(),
	).Return(&ctypes.ResultABCIInfo{
		Response: abci.ResponseInfo{
			Data:            kpki.EncodeHex(epochByt),
			LastBlockHeight: blockHeight,
		},
	}, nil)

	// initialize pki client with light client
	lc := &lcmock.LightClient{}
	rootHash, err := testOp.Run(nil)
	require.NoError(err)
	lc.On("VerifyLightBlockAtHeight", context.Background(), int64(2), mock.AnythingOfType("time.Time")).Return(
		&types.LightBlock{
			SignedHeader: &types.SignedHeader{
				Header: &types.Header{AppHash: rootHash[0]},
			},
		},
		nil,
	)

	c := lightrpc.NewClient(next, lc,
		lightrpc.KeyPathFn(func(_ string, key []byte) (merkle.KeyPath, error) {
			kp := merkle.KeyPath{}
			kp = kp.AppendKey(key, merkle.KeyEncodingURL)
			return kp, nil
		}))

	logPath := filepath.Join(testDir, "pkiclient_log")
	logBackend, err := katlog.New(logPath, "INFO", true)
	require.NoError(err)

	pkiClient, err := NewPKIClientFromLightClient(c, logBackend)
	require.NoError(err)
	require.NotNil(pkiClient)

	// test get abci info
	e := getEpoch(next, require)
	require.Equal(e, epoch)

	// test get document with pki client
	doc, _, err := pkiClient.Get(context.Background(), epoch)
	require.NoError(err)
	require.Equal(doc, testDoc)
}

// TestMockPKIClientPostTx tests PKI Client post transaction and verifies proofs.
func TestMockPKIClientPostTx(t *testing.T) {
	var (
		require        = require.New(t)
		epoch   uint64 = 1
	)

	// create a test document
	_, docSer := testutil.CreateTestDocument(require, epoch)
	testDoc, err := s11n.VerifyAndParseDocument(docSer)
	require.NoError(err)
	require.NotNil(testDoc)

	rawTx := kpki.Transaction{
		Version: kpki.ProtocolVersion,
		Epoch:   epoch,
		Command: kpki.AddConsensusDocument,
		Payload: string(docSer),
	}
	_, privKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(err)
	rawTx.AppendSignature(privKey)
	tx, err := kpki.EncodeJson(rawTx)
	require.NoError(err)

	// moke the abci broadcast commit
	next := &rpcmock.Client{}
	tmtx := make(types.Tx, len(tx))
	n := copy(tmtx, tx)
	require.Equal(len(tx), n)
	next.On(
		"BroadcastTxCommit",
		context.Background(),
		tmtx,
	).Return(&ctypes.ResultBroadcastTxCommit{
		CheckTx:   abci.ResponseCheckTx{Code: 0, GasWanted: 1},
		DeliverTx: abci.ResponseDeliverTx{Code: 0},
	}, nil)

	// initialize pki client with light client
	lc := &lcmock.LightClient{}
	require.NoError(err)

	c := lightrpc.NewClient(next, lc,
		lightrpc.KeyPathFn(func(_ string, key []byte) (merkle.KeyPath, error) {
			kp := merkle.KeyPath{}
			return kp, nil
		}))

	logPath := filepath.Join(testDir, "pkiclient_log")
	logBackend, err := katlog.New(logPath, "INFO", true)
	require.NoError(err)

	pkiClient, err := NewPKIClientFromLightClient(c, logBackend)
	require.NoError(err)
	require.NotNil(pkiClient)

	_, err = pkiClient.PostTx(context.Background(), rawTx)
	require.NoError(err)
}

// TestDeserialize tests PKI Client deserialize document.
func TestDeserialize(t *testing.T) {
	var (
		require        = require.New(t)
		epoch   uint64 = 1
	)

	// create a test document
	_, docSer := testutil.CreateTestDocument(require, epoch)
	testDoc, err := s11n.VerifyAndParseDocument(docSer)
	require.NoError(err)

	// make the abci query
	next := &rpcmock.Client{}

	// initialize pki client with light client
	lc := &lcmock.LightClient{}

	c := lightrpc.NewClient(next, lc,
		lightrpc.KeyPathFn(func(_ string, key []byte) (merkle.KeyPath, error) {
			kp := merkle.KeyPath{}
			return kp, nil
		}))

	logPath := filepath.Join(testDir, "pkiclient_log")
	logBackend, err := katlog.New(logPath, "INFO", true)
	require.NoError(err)

	pkiClient, err := NewPKIClientFromLightClient(c, logBackend)
	require.NoError(err)
	require.NotNil(pkiClient)

	doc, err := pkiClient.Deserialize(docSer)
	require.NoError(err)
	require.Equal(doc, testDoc)
}

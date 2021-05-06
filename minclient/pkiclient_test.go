package minclient

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/binary"
	"encoding/json"
	"io/ioutil"
	stdlog "log"
	"os"
	"path/filepath"
	"testing"
	"time"

	katzenmint "github.com/hashcloak/katzenmint-pki"
	"github.com/hashcloak/katzenmint-pki/s11n"
	"github.com/katzenpost/core/crypto/rand"
	katlog "github.com/katzenpost/core/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tendermint/tendermint/light"
	httpp "github.com/tendermint/tendermint/light/provider/http"
	rpcclient "github.com/tendermint/tendermint/rpc/client"
	"github.com/tendermint/tendermint/rpc/client/local"
	rpctest "github.com/tendermint/tendermint/rpc/test"
	dbm "github.com/tendermint/tm-db"
)

var (
	testDir    string
	abciClient *local.Local
)

func TestGetDocument(t *testing.T) {
	var (
		assert     = assert.New(t)
		require    = require.New(t)
		config     = rpctest.GetConfig()
		chainID    = config.ChainID()
		rpcAddress = config.RPC.ListenAddress
	)

	// Give Tendermint time to generate some blocks
	time.Sleep(3 * time.Second)

	// Get an initial trusted block
	primary, err := httpp.New(chainID, rpcAddress)
	if err != nil {
		t.Fatal(err)
	}
	block, err := primary.LightBlock(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	trustOptions := light.TrustOptions{
		Period: 10 * time.Minute,
		Height: block.Height,
		Hash:   block.Hash(),
	}

	// Setup a pki client
	logPath := filepath.Join(testDir, "pkiclient_log")
	logBackend, err := katlog.New(logPath, "INFO", false)
	if err != nil {
		t.Fatal(err)
	}
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
	if err != nil {
		t.Fatal(err)
	}

	// Get the upcoming epoch
	appInfo, err := abciClient.ABCIInfo(context.Background())
	require.Nil(err)
	infoData := katzenmint.DecodeHex(appInfo.Response.Data)
	epoch, err := binary.ReadUvarint(bytes.NewReader(infoData))
	require.Nil(err)
	epoch += 1

	// Create a document
	_, docSer := katzenmint.CreateTestDocument(require, epoch)
	docTest, err := s11n.VerifyAndParseDocument(docSer)
	require.Nil(err)
	rawTx := katzenmint.Transaction{
		Version: katzenmint.ProtocolVersion,
		Epoch:   epoch,
		Command: katzenmint.AddConsensusDocument,
		Payload: string(docSer),
	}
	_, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	rawTx.AppendSignature(privKey)
	tx, err := json.Marshal(rawTx)
	if err != nil {
		t.Fatal(err)
	}

	// Upload the document
	resp, err := abciClient.BroadcastTxCommit(context.Background(), tx)
	if err != nil {
		t.Fatal(err)
	}
	assert.True(resp.CheckTx.IsOK(), "Failed to broadcast transaction")
	assert.True(resp.DeliverTx.IsOK(), resp.DeliverTx.Log)

	// Get the document and verify
	time.Sleep(3 * time.Second)
	err = rpcclient.WaitForHeight(abciClient, resp.Height+1, nil)
	if err != nil {
		t.Fatal(err)
	}
	doc, _, err := pkiClient.Get(context.Background(), epoch)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(docTest, doc, "Got an incorrect document")

	// Try getting a non-existing document
	_, _, err = pkiClient.Get(context.Background(), epoch+1)
	assert.NotNil(err, "Got a document that should not exist")
}

func TestMain(m *testing.M) {
	var err error

	// set up test directory
	testDir, err = ioutil.TempDir("", "pkiclient_test")
	if err != nil {
		stdlog.Fatal(err)
	}

	// start katzenmint node in the background to test against
	db := dbm.NewMemDB()
	app := katzenmint.NewKatzenmintApplication(db)
	node := rpctest.StartTendermint(app, rpctest.SuppressStdout)
	abciClient = local.New(node)

	code := m.Run()

	// and shut down properly at the end
	rpctest.StopTendermint(node)
	db.Close()
	os.RemoveAll(testDir)
	os.Exit(code)
}

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
)

var (
	testDir    string
	abciClient *local.Local
)

func newDiscardLogger() (logger log.Logger) {
	logger = log.NewTMLogger(log.NewSyncWriter(ioutil.Discard))
	return
}

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
	primary, err := http.New(chainID, rpcAddress)
	assert.NoError(err)

	block, err := primary.LightBlock(context.Background(), 0)
	assert.NoError(err)

	trustOptions := light.TrustOptions{
		Period: 10 * time.Minute,
		Height: block.Height,
		Hash:   block.Hash(),
	}

	// Setup a pki client
	logPath := filepath.Join(testDir, "pkiclient_log")
	logBackend, err := katlog.New(logPath, "INFO", false)
	assert.NoError(err)

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
	assert.NoError(err)

	// Get the upcoming epoch
	appInfo, err := abciClient.ABCIInfo(context.Background())
	assert.NoError(err)
	infoData := kpki.DecodeHex(appInfo.Response.Data)
	epoch, err := binary.ReadUvarint(bytes.NewReader(infoData))
	assert.NoError(err)
	epoch += 1

	// Create a document
	_, docSer := testutil.CreateTestDocument(require, epoch)
	docTest, err := s11n.VerifyAndParseDocument(docSer)
	assert.NoError(err)
	rawTx := kpki.Transaction{
		Version: kpki.ProtocolVersion,
		Epoch:   epoch,
		Command: kpki.AddConsensusDocument,
		Payload: string(docSer),
	}
	_, privKey, err := ed25519.GenerateKey(rand.Reader)
	assert.NoError(err)

	rawTx.AppendSignature(privKey)
	tx, err := json.Marshal(rawTx)
	assert.NoError(err)

	// Upload the document
	resp, err := abciClient.BroadcastTxCommit(context.Background(), tx)
	assert.NoError(err)
	assert.True(resp.CheckTx.IsOK(), "Failed to broadcast transaction")
	assert.True(resp.DeliverTx.IsOK(), resp.DeliverTx.Log)

	// Get the document and verify
	time.Sleep(3 * time.Second)
	err = rpcclient.WaitForHeight(abciClient, resp.Height+1, nil)
	assert.NoError(err)

	doc, _, err := pkiClient.Get(context.Background(), epoch)
	assert.NoError(err)
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

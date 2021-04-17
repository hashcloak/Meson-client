package minclient

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"io/ioutil"
	stdlog "log"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dgraph-io/badger"
	katzenmint "github.com/hashcloak/katzenmint-pki"
	"github.com/hashcloak/katzenmint-pki/s11n"
	"github.com/katzenpost/core/crypto/rand"
	katlog "github.com/katzenpost/core/log"
	cpki "github.com/katzenpost/core/pki"
	"github.com/stretchr/testify/assert"
	"github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/light"
	httpp "github.com/tendermint/tendermint/light/provider/http"
	"github.com/tendermint/tendermint/rpc/client/local"
	rpctest "github.com/tendermint/tendermint/rpc/test"
)

var (
	testDir    string
	abciClient *local.Local
)

func TestGetDocument(t *testing.T) {
	t.Log("Testing PKIClient Get()")
	var (
		assert            = assert.New(t)
		config            = rpctest.GetConfig()
		chainID           = config.ChainID()
		rpcAddress        = config.RPC.ListenAddress
		preHeight  int64  = 1
		postHeight uint64 = 2
	)

	// Give Tendermint time to generate some blocks
	time.Sleep(5 * time.Second)

	// Get an initial trusted block
	primary, err := httpp.New(chainID, rpcAddress)
	if err != nil {
		t.Fatal(err)
	}
	block, err := primary.LightBlock(context.Background(), preHeight)
	if err != nil {
		t.Fatal(err)
	}
	trustOptions := light.TrustOptions{
		Period: 10 * time.Minute,
		Height: preHeight,
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

	// Push a document
	// TODO: a suitable testing document
	docTest := cpki.Document{
		Epoch: postHeight,
	}
	docJson, err := json.Marshal(docTest)
	if err != nil {
		t.Fatal(err)
	}
	rawTx := katzenmint.Transaction{
		Version: s11n.DocumentVersion,
		Epoch:   postHeight,
		Command: katzenmint.AddConsensusDocument,
		Payload: katzenmint.EncodeHex(docJson),
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
	resp, err := abciClient.BroadcastTxSync(context.Background(), tx)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(types.CodeTypeOK, resp.Code, "Failed to broadcast document")

	// Wait a while and try to get the document
	time.Sleep(5 * time.Second)
	doc, _, err := pkiClient.Get(context.Background(), postHeight)

	// Output Validation
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(docTest, doc, "Got an incorrect document")
}

func TestMain(m *testing.M) {
	var db *badger.DB
	var err error

	// set up database for katzenmint node
	testDir, err = ioutil.TempDir("", "pkiclient_test")
	if err != nil {
		stdlog.Fatal(err)
	}
	path := filepath.Join(testDir, "fullnode_db")
	db, err = badger.Open(badger.DefaultOptions(path))
	if err != nil {
		stdlog.Fatal(err)
	}

	// start katzenmint node in the background to test against
	app := katzenmint.NewKatzenmintApplication(db)
	node := rpctest.StartTendermint(app, rpctest.SuppressStdout)
	abciClient = local.New(node)

	code := m.Run()

	// and shut down properly at the end
	rpctest.StopTendermint(node)
	os.RemoveAll(testDir)
	os.Exit(code)
}

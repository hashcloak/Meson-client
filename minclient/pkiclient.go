// pkiclient.go - katzenmint implementation of katzenpost PKIClient interface

package minclient

import (
	"context"
	"crypto/ed25519"
	"fmt"

	"github.com/cosmos/iavl"
	kpki "github.com/hashcloak/katzenmint-pki"
	"github.com/hashcloak/katzenmint-pki/s11n"
	"github.com/katzenpost/core/crypto/eddsa"
	"github.com/katzenpost/core/log"
	cpki "github.com/katzenpost/core/pki"
	"github.com/tendermint/tendermint/crypto/merkle"
	"github.com/tendermint/tendermint/light"
	lightrpc "github.com/tendermint/tendermint/light/rpc"
	dbs "github.com/tendermint/tendermint/light/store/db"
	rpcclient "github.com/tendermint/tendermint/rpc/client"
	"github.com/tendermint/tendermint/rpc/client/http"
	ctypes "github.com/tendermint/tendermint/rpc/core/types"
	dbm "github.com/tendermint/tm-db"
	"gopkg.in/op/go-logging.v1"
)

type PKIClientConfig struct {
	LogBackend         *log.Backend
	ChainID            string
	TrustOptions       light.TrustOptions
	PrimaryAddress     string
	WitnessesAddresses []string
	DatabaseName       string
	DatabaseDir        string
	RpcAddress         string
}

type PKIClient struct {
	// TODO: do we need katzenpost pki client interface?
	// cpki.Client
	light *lightrpc.Client
	log   *logging.Logger
}

// Get returns the PKI document along with the raw serialized form for the provided epoch.
func (p *PKIClient) Get(ctx context.Context, epoch uint64) (*cpki.Document, []byte, error) {
	p.log.Debugf("Get(ctx, %d)", epoch)

	// Form the abci query
	query := kpki.Query{
		Version: kpki.ProtocolVersion,
		Epoch:   epoch,
		Command: kpki.GetConsensus,
		Payload: "",
	}
	data, err := kpki.EncodeJson(query)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to encode data: %v", err)
	}
	p.log.Debugf("Query: %v", query)

	// Make the abci query
	opts := rpcclient.ABCIQueryOptions{Prove: true}
	resp, err := p.light.ABCIQueryWithOptions(ctx, "", data, opts)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to query katzenmint pki: %v", err)
	}

	// Check for response status
	if resp.Response.Code != 0 {
		return nil, nil, cpki.ErrNoDocument
	}

	// Verify and parse the document
	doc, err := s11n.VerifyAndParseDocument(resp.Response.Value)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to extract doc: %v", err)
	}
	if doc.Epoch != epoch {
		p.log.Warningf("Get() returned pki document for wrong epoch: %v", doc.Epoch)
		return nil, nil, s11n.ErrInvalidEpoch
	}
	p.log.Debugf("Document: %v", doc)

	return doc, resp.Response.Value, nil
}

// Post posts the node's descriptor to the PKI for the provided epoch.
func (p *PKIClient) Post(ctx context.Context, epoch uint64, signingKey *eddsa.PrivateKey, d *cpki.MixDescriptor) error {
	p.log.Debugf("Post(ctx, %d, %v, %+v)", epoch, signingKey.PublicKey(), d)

	// Ensure that the descriptor we are about to post is well formed.
	if err := s11n.IsDescriptorWellFormed(d, epoch); err != nil {
		return err
	}

	// Make a serialized + signed + serialized descriptor.
	signed, err := s11n.SignDescriptor(signingKey, d)
	if err != nil {
		return err
	}
	p.log.Debugf("Signed descriptor: '%v'", signed)

	// Form the abci transaction
	rawTx := kpki.Transaction{
		Version: "1",
		Epoch:   epoch,
		Command: kpki.PublishMixDescriptor,
		Payload: kpki.EncodeHex(signed),
	}
	// ! Problemsome here
	rawTx.AppendSignature(ed25519.PrivateKey(signingKey.Bytes()))
	tx, err := kpki.EncodeJson(rawTx)
	if err != nil {
		return err
	}
	p.log.Debugf("Transaction: '%v'", tx)

	// Broadcast the abci transaction
	resp, err := p.light.BroadcastTxSync(ctx, tx)
	if err != nil {
		return err
	}
	if resp.Code != 0 {
		return fmt.Errorf("broadcast Tx returned with status code: %v", resp.Code)
	}
	// TODO: Make sure to subscribe for events

	// Parse the post_descriptor_status command.
	/*
		r, ok := resp.(*commands.PostDescriptorStatus)
		if !ok {
			return fmt.Errorf("nonvoting/client: Post() unexpected reply: %T", resp)
		}
		switch r.ErrorCode {
		case commands.DescriptorOk:
			return nil
		case commands.DescriptorConflict:
			return cpki.ErrInvalidPostEpoch
		default:
			return fmt.Errorf("nonvoting/client: Post() rejected by authority: %v", postErrorToString(r.ErrorCode))
		}
	*/
	return nil
}

// PostTx posts the transaction to the katzenmint node.
func (p *PKIClient) PostTx(ctx context.Context, tx kpki.Transaction) (*ctypes.ResultBroadcastTxCommit, error) {
	p.log.Debugf("PostTx(ctx, %d)", tx.Epoch)

	if !tx.IsVerified() {
		return nil, fmt.Errorf("transaction is not verified, did you forget signing?")
	}

	encTx, err := kpki.EncodeJson(tx)
	if err != nil {
		return nil, err
	}
	p.log.Debugf("Transaction: '%v'", tx)

	// Broadcast the abci transaction
	resp, err := p.light.BroadcastTxCommit(ctx, encTx)
	if err != nil {
		return resp, err
	}
	if !resp.CheckTx.IsOK() {
		return resp, fmt.Errorf("send transaction failed at checking tx")
	}
	if !resp.DeliverTx.IsOK() {
		return resp, fmt.Errorf("send transaction failed at delivering tx")
	}
	return resp, nil
}

// Deserialize returns PKI document given the raw bytes.
func (p *PKIClient) Deserialize(raw []byte) (*cpki.Document, error) {
	return s11n.VerifyAndParseDocument(raw)
}

// NewPKIClient create PKI Client from PKI config
func NewPKIClient(cfg *PKIClientConfig) (*PKIClient, error) {
	p := new(PKIClient)
	p.log = cfg.LogBackend.GetLogger("pki/client")

	db, err := dbm.NewDB(cfg.DatabaseName, dbm.GoLevelDBBackend, cfg.DatabaseDir)
	if err != nil {
		return nil, fmt.Errorf("Error opening katzenmint-pki database: %v", err)
	}
	lightclient, err := light.NewHTTPClient(
		context.Background(),
		cfg.ChainID,
		cfg.TrustOptions,
		cfg.PrimaryAddress,
		cfg.WitnessesAddresses,
		dbs.New(db, "katzenmint"),
	)
	if err != nil {
		return nil, fmt.Errorf("Error initialization of katzenmint-pki light client: %v", err)
	}
	provider, err := http.New(cfg.RpcAddress, "/websocket")
	if err != nil {
		return nil, fmt.Errorf("Error connection to katzenmint-pki full node: %v", err)
	}
	kpFunc := lightrpc.KeyPathFn(func(_ string, key []byte) (merkle.KeyPath, error) {
		kp := merkle.KeyPath{}
		kp = kp.AppendKey(key, merkle.KeyEncodingURL)
		return kp, nil
	})
	p.light = lightrpc.NewClient(provider, lightclient, kpFunc)
	p.light.RegisterOpDecoder(iavl.ProofOpIAVLValue, iavl.ValueOpDecoder)
	return p, nil
}

// NewPKIClientFromLightClient create PKI Client from tendermint rpc light client
func NewPKIClientFromLightClient(light *lightrpc.Client, logBackend *log.Backend) (cpki.Client, error) {
	p := new(PKIClient)
	p.log = logBackend.GetLogger("pki/client")
	p.light = light
	p.light.RegisterOpDecoder(iavl.ProofOpIAVLValue, iavl.ValueOpDecoder)
	return p, nil
}

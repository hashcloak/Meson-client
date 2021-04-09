// pkiclient.go - katzenmint implementation of katzenpost PKIClient interface

package minclient

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"

	kpki "github.com/hashcloak/katzenmint-pki"
	"github.com/hashcloak/katzenmint-pki/s11n"
	"github.com/katzenpost/core/crypto/eddsa"
	"github.com/katzenpost/core/log"
	cpki "github.com/katzenpost/core/pki"
	"github.com/tendermint/tendermint/light"
	lightrpc "github.com/tendermint/tendermint/light/rpc"
	dbs "github.com/tendermint/tendermint/light/store/db"
	"github.com/tendermint/tendermint/rpc/client/http"
	"github.com/tendermint/tm-db/badgerdb"
	"gopkg.in/op/go-logging.v1"
)

var _ cpki.Client = (*PKIClient)(nil)

type PKIClientConfig struct {
	LogBackend         *log.Backend
	TrustOptions       light.TrustOptions
	PrimaryAddress     string
	WitnessesAddresses []string
	DatabaseName       string
	DatabaseDir        string
	Rpcaddress         string
}

type PKIClient struct {
	light *lightrpc.Client
	log   *logging.Logger
}

func (p *PKIClient) Get(ctx context.Context, epoch uint64) (*cpki.Document, []byte, error) {
	p.log.Debugf("Get(ctx, %d)", epoch)

	// Form the abci query
	query, err := json.Marshal(kpki.Query{
		Version: "1",
		Epoch:   epoch,
		Command: kpki.GetConsensus,
		Payload: "",
	})
	if err != nil {
		return nil, nil, err
	}
	p.log.Debugf("Query: %v", query)

	// Make the abci query
	resp, err := p.light.ABCIQuery(ctx, "TODO: /path", query)
	if err != nil {
		return nil, nil, err
	}

	// Check for response status
	if resp.Response.Code != 0 {
		return nil, nil, cpki.ErrNoDocument
	}

	// Verify and parse the document
	doc, err := s11n.VerifyAndParseDocument(resp.Response.Value, nil)
	if err != nil {
		return nil, nil, err
	}
	if doc.Epoch != epoch {
		p.log.Warningf("Get() returned pki document for wrong epoch: %v", doc.Epoch)
		return nil, nil, s11n.ErrInvalidEpoch
	}
	p.log.Debugf("Document: %v", doc)

	return doc, resp.Response.Value, nil
}

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
	tx, err := json.Marshal(rawTx)
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
		return fmt.Errorf("Broadcast Tx returned with status code: %v", resp.Code)
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

func (p *PKIClient) Deserialize(raw []byte) (*cpki.Document, error) {
	return s11n.VerifyAndParseDocument(raw, nil)
}

func NewPKIClient(cfg *PKIClientConfig) (cpki.Client, error) {
	p := new(PKIClient)
	p.log = cfg.LogBackend.GetLogger("pki/client")

	db, err := badgerdb.NewDB(cfg.DatabaseName, cfg.DatabaseDir)
	if err != nil {
		p.log.Errorf("Error opening katzenmint-pki database.")
	}
	lightclient, err := light.NewHTTPClient(
		context.Background(),
		"TODO: chainID_of_katzenmint_pki",
		cfg.TrustOptions,
		cfg.PrimaryAddress,
		cfg.WitnessesAddresses,
		dbs.New(db, "katzenmint"),
	)
	if err != nil {
		p.log.Errorf("Error initialization of katzenmint-pki light client.")
		return nil, err
	}
	provider, err := http.New(cfg.Rpcaddress, "/websocket")
	if err != nil {
		p.log.Errorf("Error connection to katzenmint-pki full node.")
		return nil, err
	}

	p.light = lightrpc.NewClient(provider, lightclient)
	return p, nil
}

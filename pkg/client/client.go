package client

import (
	"fmt"
	"errors"
	"sync"
	"time"

	"github.com/katzenpost/core/crypto/ecdh"
	"github.com/katzenpost/core/crypto/rand"
	"github.com/katzenpost/client"
	"github.com/katzenpost/client/config"
	"github.com/katzenpost/core/log"
	"github.com/hashcloak/Meson/plugin/pkg/common"
	"gopkg.in/op/go-logging.v1"
)

type Client struct {
	*client.Client
	cfg        *config.Config
	logBackend *log.Backend
	log        *logging.Logger
	fatalErrCh chan error
	haltedCh   chan interface{}
	haltOnce   *sync.Once

	session 	*client.Session
	linkKey		*ecdh.PrivateKey
	service		string
}


func (c *Client) Start() {
	// Retrieve PKI consensus documents and related info
	cfg, linkKey := client.AutoRegisterRandomClient(c.cfg)
	c.cfg = cfg
	c.linkKey = linkKey
	session, err := c.NewSession(c.linkKey)
	if err != nil {
		panic(err)
	}
	c.session = session
}

func (c *Client) Stop() {
	// shutdown client and clean up any threads
	c.Shutdown()
}

func (c *Client) SendRawTransaction(rawTransactionBlob *string, chainID *int, ticker *string) ([]byte, error) {
	// Send a raw transaction blob to a provider
	// Pretty much the same code that's in main.go
	defer c.Stop()

	req := common.NewRequest(*ticker, *rawTransactionBlob, *chainID)
	mesonRequest := req.ToJson()

	mesonService, err := c.session.GetService(c.service)
	if err != nil {
		return nil, err
	}

	reply, err := c.session.BlockingSendUnreliableMessage(mesonService.Name, mesonService.Provider, mesonRequest)
	if err != nil { 
		return nil, err
	}

	return reply, nil
}

// Creates a new Meson Client with the provided configuration
func New(cfg *config.Config, service string) (*Client, error) {
	client := &Client{
		cfg: cfg,
		fatalErrCh: make(chan error),
		haltedCh:	make(chan interface{}),
		haltOnce: new(sync.Once),
		linkKey: new(ecdh.PrivateKey),
		service: service,
	}

	return client, nil
}
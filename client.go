// Package client provides a thin-wrapper of the Katzenpost client library
// for cryptocurrency transactions.
package client

import (
	"errors"
	"path/filepath"
	"sync"

	"github.com/hashcloak/Meson-plugin/pkg/common"
	katzenClient "github.com/katzenpost/client"
	"github.com/katzenpost/client/config"
	"github.com/katzenpost/core/crypto/ecdh"
	"github.com/katzenpost/core/log"
	"gopkg.in/op/go-logging.v1"
)

type Client struct {
	*katzenClient.Client

	cfg        *config.Config
	logBackend *log.Backend
	log        *logging.Logger
	fatalErrCh chan error
	haltedCh   chan interface{}
	haltOnce   *sync.Once
	session    *katzenClient.Session
	linkKey    *ecdh.PrivateKey
	service    string
}

// Start begins a Meson client.
// The client retrieves PKI consensus documents in order to get a view of the network
// and connect to a provider.
// It returns an error if they were any issues starting the client.
func (c *Client) Start() error {
	var err error
	// Retrieve PKI consensus documents and related info
	_, c.linkKey = katzenClient.AutoRegisterRandomClient(c.cfg)
	c.session, err = c.NewSession(c.linkKey)
	return err
}

// SendRawTransaction takes a signed transaction blob, a destination blockchain
// along with its ticker symbol and sends that blob to a provider that will
// send the blob to the right blockchain.
// It returns a reply and any error encountered.

// Note: This is subject to change as we add more support for other blockchains
func (c *Client) SendRawTransaction(rawTransactionBlob *string, ticker *string) ([]byte, error) {
	defer c.Shutdown()

	req := common.NewRequest(*ticker, *rawTransactionBlob)
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

// InitLogging provides logging for the meson client
// It returns any errors it encounters.
func (c *Client) InitLogging() error {
	f := c.cfg.Logging.File
	if !c.cfg.Logging.Disable && c.cfg.Logging.File != "" {
		if !filepath.IsAbs(f) {
			return errors.New("log file path must be absolute path")
		}
	}

	var err error
	c.logBackend, err = log.New(f, c.cfg.Logging.Level, c.cfg.Logging.Disable)
	if err == nil {
		c.log = c.logBackend.GetLogger("katzenpost/client")
	}
	return err
}

// New instantiates a new Meson client with the provided configuration file
// and service that represents the chain it's being used for.
// It returns a Client struct pointer and any errors encountered.
func New(cfgFile string, service string) (*Client, error) {
	cfg, err := config.LoadFile(cfgFile)
	if err != nil {
		return nil, err
	}

	katzen, err := katzenClient.New(cfg)
	if err != nil {
		return nil, err
	}

	client := &Client{
		Client:     katzen,
		cfg:        cfg,
		fatalErrCh: make(chan error),
		haltedCh:   make(chan interface{}),
		haltOnce:   new(sync.Once),
		linkKey:    new(ecdh.PrivateKey),
		service:    service,
	}

	if err := client.InitLogging(); err != nil {
		return nil, err
	}

	// Start the fatal error watcher.
	go func() {
		err, ok := <-client.fatalErrCh
		if !ok {
			return
		}
		client.log.Warningf("Shutting down due to error: %v", err)
		client.Shutdown()
	}()

	return client, nil
}

package client

import (
	"errors"
	"github.com/hashcloak/Meson/plugin/pkg/common"
	katzenClient "github.com/katzenpost/client"
	"github.com/katzenpost/client/config"
	"github.com/katzenpost/core/crypto/ecdh"
	"github.com/katzenpost/core/log"
	"gopkg.in/op/go-logging.v1"
	"path/filepath"
	"sync"
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

func (c *Client) Start() error {
	var err error
	// Retrieve PKI consensus documents and related info
	_, c.linkKey = katzenClient.AutoRegisterRandomClient(c.cfg)
	c.session, err = c.NewSession(c.linkKey)
	return err
}

func (c *Client) SendRawTransaction(rawTransactionBlob *string, chainID *int, ticker *string) ([]byte, error) {
	// Send a raw transaction blob to a provider
	// Pretty much the same code that's in main.go
	defer c.Shutdown()

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

// Creates a new Meson Client with the provided configuration
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

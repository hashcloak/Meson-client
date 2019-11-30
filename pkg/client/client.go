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
	linkKey		*ecdh.PrivateKey
	service		string
}

type ethRequest struct {
	Version int
	ChainID int
	Tx		string
}

func (c *Client) Start() error {
	// Uses some of the code from main.go with better threading and error handling
	// Retrieve PKI consensus documents and related info
	//cfg, linkKey := client.AutoRegisterRandomClient(c.Client.cfg)

	// Periodically send decoy messages to providers

}

func (c *Client) Stop() {
	// shutdown client and clean up any threads
	c.Shutdown()
}

func (c *Client) SendRawTransaction(rawTransactionBlob *string, chainID *int, ticker *string) error {
	// Send a raw transaction blob to a provider
	// Pretty much the same code that's in main.go


}

// Creates a new Meson Client with the provided configuration
func New(cfg *config.Config, service string) (*Client, error) {
	c, err := client.New(cfg)
	if err != nil {
		panic(err)
	}

	client := &Client{
		c,
		new(ecdh.PrivateKey),
		service,
	}

	return client, nil
}
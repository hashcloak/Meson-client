package client

import (
	"fmt"
	"errors"
	"sync"
	"time"

	"github.com/katzenpost/core/crypto/ecdh"
	"github.com/katzenpost/core/crypto/rand"
	"github.com/katzenpost/client"
	"github.com/katzenpost/core/log"
	"github.com/hashcloak/Meson/plugin/pkg/common"
)

type Client struct {
	client  	*client.Client
	session 	*client.Session
	log			*logging.logger
	logBackend  *log.Backend

	linkKey *ecdh.PrivateKey
}

type ethRequest struct {
	Version int
	ChainID int
	Tx		string
}

func (c *Client) Start(cfg *config.Config) error {
	// Uses some of the code from main.go with better threading and error handling
	// Retrieve PKI consensus documents and related info
	c.AutoRegisterRandomClient()

	// Periodically send decoy messages to providers

}

func (c *Client) Stop() error {
	// shutdown client and clean up any threads
	c.shutdown()
}

func (c *Client) SendRawTransaction() error {
	// Send a raw transaction blob to a provider
	// Pretty much the same code that's in main.go

}

func New() (*Client, error) {
	// Initialize new client
}
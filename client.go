// Package client provides a thin-wrapper of the Katzenpost client library
// for cryptocurrency transactions.
package client

import (
	"context"
	"errors"
	"fmt"
	mrand "math/rand"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/hashcloak/Meson-client/config"
	"github.com/hashcloak/Meson-client/pkiclient/epochtime"
	"github.com/hashcloak/Meson-plugin/pkg/common"
	kClient "github.com/katzenpost/client"
	"github.com/katzenpost/core/crypto/ecdh"
	"github.com/katzenpost/core/crypto/rand"
	"github.com/katzenpost/core/log"
	"github.com/katzenpost/core/pki"
	registration "github.com/katzenpost/registration_client"
	"gopkg.in/op/go-logging.v1"
)

const (
	initialPKIConsensusTimeout = 45 * time.Second
)

func AutoRegisterRandomClient(cfg *config.Config) (*config.Config, *ecdh.PrivateKey) {
	// Retrieve a copy of the PKI consensus document.
	logFilePath := ""
	backendLog, err := log.New(logFilePath, "DEBUG", false)
	if err != nil {
		panic(err)
	}
	proxyCfg := cfg.UpstreamProxyConfig()
	pkiClient, err := cfg.NewPKIClient(backendLog, proxyCfg)
	if err != nil {
		panic(err)
	}
	// have to shutdown pkiclient and release database
	// maybe find better solution?
	defer pkiClient.Shutdown()
	currentEpoch, _, _, err := epochtime.Now(pkiClient)
	if err != nil {
		panic(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), initialPKIConsensusTimeout)
	defer cancel()
	doc, _, err := pkiClient.GetDoc(ctx, currentEpoch)
	if err != nil {
		panic(err)
	}

	// Pick a registration Provider.
	registerProviders := []*pki.MixDescriptor{}
	for _, provider := range doc.Providers {
		if provider.RegistrationHTTPAddresses != nil {
			registerProviders = append(registerProviders, provider)
		}
	}
	if len(registerProviders) == 0 {
		panic("zero registration Providers found in the consensus")
	}
	mrand.Seed(time.Now().UTC().UnixNano())
	registrationProvider := registerProviders[mrand.Intn(len(registerProviders))]

	// Register with that Provider.
	fmt.Println("registering client with mixnet Provider")
	linkKey, err := ecdh.NewKeypair(rand.Reader)
	if err != nil {
		panic(err)
	}
	account := &config.Account{
		User:           fmt.Sprintf("%x", linkKey.PublicKey().Bytes()),
		Provider:       registrationProvider.Name,
		ProviderKeyPin: registrationProvider.IdentityKey,
	}

	u, err := url.Parse(registrationProvider.RegistrationHTTPAddresses[0])
	if err != nil {
		panic(err)
	}
	cfgRegistration := &config.Registration{
		Address: u.Host,
		Options: &registration.Options{
			Scheme:       u.Scheme,
			UseSocks:     strings.HasPrefix(cfg.UpstreamProxy.Type, "socks"),
			SocksNetwork: cfg.UpstreamProxy.Network,
			SocksAddress: cfg.UpstreamProxy.Address,
		},
	}
	cfg.Account = account
	cfg.Registration = cfgRegistration
	err = RegisterClient(cfg, linkKey.PublicKey())
	if err != nil {
		panic(err)
	}
	return cfg, linkKey
}

func RegisterClient(cfg *config.Config, linkKey *ecdh.PublicKey) error {
	client, err := registration.New(cfg.Registration.Address, cfg.Registration.Options)
	if err != nil {
		return err
	}
	err = client.RegisterAccountWithLinkKey(cfg.Account.User, linkKey)
	return err
}

type Client struct {
	*kClient.Client

	cfg        *config.Config
	logBackend *log.Backend
	log        *logging.Logger
	fatalErrCh chan error
	haltedCh   chan interface{}
	haltOnce   *sync.Once
	session    *Session
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
	_, c.linkKey = AutoRegisterRandomClient(c.cfg)
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
		c.log = c.logBackend.GetLogger("hashcloak/Meson-client")
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

	// katzen, err := kClient.New(cfg)
	// if err != nil {
	// 	return nil, err
	// }

	client := &Client{
		// Client:     katzen,
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

// New establishes a session with provider using key.
// This method will block until session is connected to the Provider.
func (c *Client) NewSession(linkKey *ecdh.PrivateKey) (*Session, error) {
	var err error
	timeout := time.Duration(c.cfg.Debug.SessionDialTimeout) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	c.session, err = NewSession(ctx, c.fatalErrCh, c.logBackend, c.cfg, linkKey)
	return c.session, err
}

// Package client provides a Go Client for the IPFS Cluster API provided
// by the "api/rest" component. It supports both the HTTP(s) endpoint and
// the libp2p-http endpoint.
package api

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	httpapi "github.com/ipfs/go-ipfs-http-client"

	"github.com/ipfs/go-cid"
	files "github.com/ipfs/go-ipfs-files"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/multiformats/go-multiaddr"
	madns "github.com/multiformats/go-multiaddr-dns"
	manet "github.com/multiformats/go-multiaddr-net"

	"go.opencensus.io/plugin/ochttp"
	"go.opencensus.io/plugin/ochttp/propagation/tracecontext"
	"go.opencensus.io/trace"
)

// Configuration defaults
var (
	DefaultTimeout   = 0
	DefaultAPIAddr   = "/ip4/127.0.0.1/tcp/9094"
	DefaultLogLevel  = "info"
	DefaultProxyPort = 9095
	ResolveTimeout   = 30 * time.Second
	DefaultPort      = 9094
)

// Client interface defines the interface to be used by API clients to
// interact with the ipfs-cluster-service. All methods take a
// context.Context as their first parameter, this allows for
// timing out and cancelling of requests as well as recording
// metrics and tracing of requests through the API.
type Client interface {
	// ID returns information about the cluster Peer.
	ID(context.Context) (*ID, error)

	// Peers requests ID information for all cluster peers.
	Peers(context.Context) ([]*ID, error)
	// PeerJoin add a peer join to the cluster.
	PeerJoin(ctx context.Context, addr string) (*ID, error)
	// PeerAdd adds a new peer to the cluster.
	PeerAdd(ctx context.Context, pid peer.ID) (*ID, error)
	// PeerRm removes a current peer from the cluster
	PeerRm(ctx context.Context, pid peer.ID) error

	// Add imports files to the cluster from the given paths.
	Add(ctx context.Context, paths []string, params *AddParams, out chan<- *AddedOutput) error
	// AddMultiFile imports new files from a MultiFileReader.
	AddMultiFile(ctx context.Context, multiFileR *files.MultiFileReader, params *AddParams, out chan<- *AddedOutput) error

	// Pin tracks a Cid with the given replication factor and a name for
	// human-friendliness.
	Pin(ctx context.Context, ci cid.Cid, opts PinOptions) (*Pin, error)
	// Unpin untracks a Cid from cluster.
	Unpin(ctx context.Context, ci cid.Cid) (*Pin, error)

	// PinPath resolves given path into a cid and performs the pin operation.
	PinPath(ctx context.Context, path string, opts PinOptions) (*Pin, error)
	// UnpinPath resolves given path into a cid and performs the unpin operation.
	// It returns Pin of the given cid before it is unpinned.
	UnpinPath(ctx context.Context, path string) (*Pin, error)

	// Allocations returns the consensus state listing all tracked items
	// and the peers that should be pinning them.
	Allocations(ctx context.Context, filter PinType) ([]*Pin, error)
	// Allocation returns the current allocations for a given Cid.
	Allocation(ctx context.Context, ci cid.Cid) (*Pin, error)

	// Status returns the current ipfs state for a given Cid. If local is true,
	// the information affects only the current peer, otherwise the information
	// is fetched from all cluster peers.
	Status(ctx context.Context, ci cid.Cid, local bool) (*GlobalPinInfo, error)
	// StatusAll gathers Status() for all tracked items.
	StatusAll(ctx context.Context, filter TrackerStatus, local bool) ([]*GlobalPinInfo, error)

	// Sync makes sure the state of a Cid corresponds to the state reported
	// by the ipfs daemon, and returns it. If local is true, this operation
	// only happens on the current peer, otherwise it happens on every
	// cluster peer.
	Sync(ctx context.Context, ci cid.Cid, local bool) (*GlobalPinInfo, error)
	// SyncAll triggers Sync() operations for all tracked items. It only
	// returns informations for items that were de-synced or have an error
	// state. If local is true, the operation is limited to the current
	// peer. Otherwise it happens on every cluster peer.
	SyncAll(ctx context.Context, local bool) ([]*GlobalPinInfo, error)

	// Recover retriggers pin or unpin ipfs operations for a Cid in error
	// state.  If local is true, the operation is limited to the current
	// peer, otherwise it happens on every cluster peer.
	Recover(ctx context.Context, ci cid.Cid, local bool) (*GlobalPinInfo, error)
	// RecoverAll triggers Recover() operations on all tracked items. If
	// local is true, the operation is limited to the current peer.
	// Otherwise, it happens everywhere.
	RecoverAll(ctx context.Context, local bool) ([]*GlobalPinInfo, error)

	// Version returns the ipfs-cluster peer's version.
	Version(context.Context) (*Version, error)

	// IPFS returns an instance of go-ipfs-api's Shell, pointing to a
	// Cluster's IPFS proxy endpoint.
	IPFS(context.Context) *httpapi.HttpApi

	// GetConnectGraph returns an ipfs-cluster connection graph.
	GetConnectGraph(context.Context) (*ConnectGraph, error)

	// Metrics returns a map with the latest metrics of matching name
	// for the current cluster peers.
	Metrics(ctx context.Context, name string) ([]*Metric, error)
}

// Config allows to configure the parameters to connect
// to the ipfs-cluster REST API.
type Config struct {
	// Enable SSL support. Only valid without APIAddr.
	SSL bool
	// Skip certificate verification (insecure)
	NoVerifyCert bool

	// Username and password for basic authentication
	Username string
	Password string

	// The ipfs-cluster REST API endpoint in multiaddress form
	// (takes precedence over host:port). It this address contains
	// an /ipfs/, /p2p/ or /dnsaddr, the API will be contacted
	// through a libp2p tunnel, thus getting encryption for
	// free. Using the libp2p tunnel will ignore any configurations.
	APIAddr multiaddr.Multiaddr

	// REST API endpoint host and port. Only valid without
	// APIAddr.
	Host string
	Port string

	// If APIAddr is provided, and the peer uses private networks (pnet),
	// then we need to provide the key. If the peer is the cluster peer,
	// this corresponds to the cluster secret.
	ProtectorKey []byte

	// ProxyAddr is used to obtain a go-ipfs-api Shell instance pointing
	// to the ipfs proxy endpoint of ipfs-cluster. If empty, the location
	// will be guessed from one of APIAddr/Host,
	// and the port used will be ipfs-cluster's proxy default port (9095)
	ProxyAddr multiaddr.Multiaddr

	// Define timeout for network operations
	Timeout time.Duration

	// Specifies if we attempt to re-use connections to the same
	// hosts.
	DisableKeepAlives bool

	// LogLevel defines the verbosity of the logging facility
	LogLevel string
}

// DefaultClient provides methods to interact with the ipfs-cluster API. Use
// NewDefaultClient() to create one.
type defaultCluster struct {
	ctx       context.Context
	cancel    context.CancelFunc
	config    *Config
	transport *http.Transport
	net       string
	hostname  string
	client    *http.Client
	p2p       host.Host
	addr      multiaddr.Multiaddr
}

// NewDefaultClient initializes a client given a Config.
func NewDefaultClient(cfg *Config) (Client, error) {
	ctx := context.Background()
	client := &defaultCluster{
		ctx:    ctx,
		config: cfg,
	}

	if client.config.Port == "" {
		client.config.Port = fmt.Sprintf("%d", DefaultPort)
	}

	err := client.setupAPIAddr()
	if err != nil {
		return nil, err
	}

	err = client.resolveAPIAddr()
	if err != nil {
		return nil, err
	}

	err = client.setupHTTPClient()
	if err != nil {
		return nil, err
	}

	err = client.setupHostname()
	if err != nil {
		return nil, err
	}

	err = client.setupProxy()
	if err != nil {
		return nil, err
	}

	return client, nil
}

func (c *defaultCluster) setupAPIAddr() error {
	if c.config.APIAddr != nil {
		return nil // already setup by user
	}

	var addr multiaddr.Multiaddr
	var err error

	if c.config.Host == "" { //default
		addr, err := multiaddr.NewMultiaddr(DefaultAPIAddr)
		c.config.APIAddr = addr
		return err
	}

	var addrStr string
	ip := net.ParseIP(c.config.Host)
	switch {
	case ip == nil:
		addrStr = fmt.Sprintf("/dns4/%s/tcp/%s", c.config.Host, c.config.Port)
	case ip.To4() != nil:
		addrStr = fmt.Sprintf("/ip4/%s/tcp/%s", c.config.Host, c.config.Port)
	default:
		addrStr = fmt.Sprintf("/ip6/%s/tcp/%s", c.config.Host, c.config.Port)
	}

	addr, err = multiaddr.NewMultiaddr(addrStr)
	c.config.APIAddr = addr
	return err
}

func (c *defaultCluster) resolveAPIAddr() error {
	// Only resolve libp2p addresses. For HTTP addresses, we let
	// the default client handle any resolving. We extract the hostname
	// in setupHostname()
	if !IsPeerAddress(c.config.APIAddr) {
		return nil
	}
	resolveCtx, cancel := context.WithTimeout(c.ctx, ResolveTimeout)
	defer cancel()
	resolved, err := madns.Resolve(resolveCtx, c.config.APIAddr)
	if err != nil {
		return err
	}

	if len(resolved) == 0 {
		return fmt.Errorf("resolving %s returned 0 results", c.config.APIAddr)
	}

	c.config.APIAddr = resolved[0]
	return nil
}

func (c *defaultCluster) setupHTTPClient() error {
	var err error

	switch {
	case IsPeerAddress(c.config.APIAddr):
		err = c.enableLibp2p()
	case c.config.SSL:
		err = c.enableTLS()
	default:
		c.defaultTransport()
	}

	if err != nil {
		return err
	}

	c.client = &http.Client{
		Transport: &ochttp.Transport{
			Base:           c.transport,
			Propagation:    &tracecontext.HTTPFormat{},
			StartOptions:   trace.StartOptions{SpanKind: trace.SpanKindClient},
			FormatSpanName: func(req *http.Request) string { return req.Host + ":" + req.URL.Path + ":" + req.Method },
			NewClientTrace: ochttp.NewSpanAnnotatingClientTrace,
		},
		Timeout: c.config.Timeout,
	}
	return nil
}

func (c *defaultCluster) setupHostname() error {
	// Extract host:port form APIAddr or use Host:Port.
	// For libp2p, hostname is set in enableLibp2p()
	if IsPeerAddress(c.config.APIAddr) {
		return nil
	}
	_, hostname, err := manet.DialArgs(c.config.APIAddr)
	if err != nil {
		return err
	}
	c.hostname = hostname
	return nil
}

func (c *defaultCluster) setupProxy() error {
	if c.config.ProxyAddr != nil {
		return nil
	}

	// Guess location from	APIAddr
	port, err := multiaddr.NewMultiaddr(fmt.Sprintf("/tcp/%d", DefaultProxyPort))
	if err != nil {
		return err
	}
	c.config.ProxyAddr = multiaddr.Split(c.config.APIAddr)[0].Encapsulate(port)
	return nil
}

// IPFS returns an instance of go-ipfs-api's Shell, pointing to the
// configured ProxyAddr (or to the default Cluster's IPFS proxy port).
// It re-uses this Client's HTTP client, thus will be constrained by
// the same configurations affecting it (timeouts...).
func (c *defaultCluster) IPFS(ctx context.Context) *httpapi.HttpApi {
	cli, err := httpapi.NewApiWithClient(c.addr, c.client)
	if err != nil {
		return nil
	}
	return cli
}

// IsPeerAddress detects if the given multiaddress identifies a libp2p peer,
// either because it has the /p2p/ protocol or because it uses /dnsaddr/
func IsPeerAddress(addr multiaddr.Multiaddr) bool {
	if addr == nil {
		return false
	}
	pid, err := addr.ValueForProtocol(multiaddr.P_P2P)
	dnsaddr, err2 := addr.ValueForProtocol(madns.DnsaddrProtocol.Code)
	return (pid != "" && err == nil) || (dnsaddr != "" && err2 == nil)
}

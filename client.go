package api

import (
	"context"

	"github.com/glvd/cluster/api"
	"github.com/ipfs/go-cid"
	files "github.com/ipfs/go-ipfs-files"
	"github.com/ipfs/go-ipfs-http-client"
	"github.com/libp2p/go-libp2p-core/peer"
)

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
	Add(ctx context.Context, paths []string, params *api.AddParams, out chan<- *api.AddedOutput) error
	// AddMultiFile imports new files from a MultiFileReader.
	AddMultiFile(ctx context.Context, multiFileR *files.MultiFileReader, params *api.AddParams, out chan<- *api.AddedOutput) error

	// Pin tracks a Cid with the given replication factor and a name for
	// human-friendliness.
	Pin(ctx context.Context, ci cid.Cid, opts api.PinOptions) (*api.Pin, error)
	// Unpin untracks a Cid from cluster.
	Unpin(ctx context.Context, ci cid.Cid) (*api.Pin, error)

	// PinPath resolves given path into a cid and performs the pin operation.
	PinPath(ctx context.Context, path string, opts api.PinOptions) (*api.Pin, error)
	// UnpinPath resolves given path into a cid and performs the unpin operation.
	// It returns api.Pin of the given cid before it is unpinned.
	UnpinPath(ctx context.Context, path string) (*api.Pin, error)

	// Allocations returns the consensus state listing all tracked items
	// and the peers that should be pinning them.
	Allocations(ctx context.Context, filter api.PinType) ([]*api.Pin, error)
	// Allocation returns the current allocations for a given Cid.
	Allocation(ctx context.Context, ci cid.Cid) (*api.Pin, error)

	// Status returns the current ipfs state for a given Cid. If local is true,
	// the information affects only the current peer, otherwise the information
	// is fetched from all cluster peers.
	Status(ctx context.Context, ci cid.Cid, local bool) (*api.GlobalPinInfo, error)
	// StatusAll gathers Status() for all tracked items.
	StatusAll(ctx context.Context, filter api.TrackerStatus, local bool) ([]*api.GlobalPinInfo, error)

	// Sync makes sure the state of a Cid corresponds to the state reported
	// by the ipfs daemon, and returns it. If local is true, this operation
	// only happens on the current peer, otherwise it happens on every
	// cluster peer.
	Sync(ctx context.Context, ci cid.Cid, local bool) (*api.GlobalPinInfo, error)
	// SyncAll triggers Sync() operations for all tracked items. It only
	// returns informations for items that were de-synced or have an error
	// state. If local is true, the operation is limited to the current
	// peer. Otherwise it happens on every cluster peer.
	SyncAll(ctx context.Context, local bool) ([]*api.GlobalPinInfo, error)

	// Recover retriggers pin or unpin ipfs operations for a Cid in error
	// state.  If local is true, the operation is limited to the current
	// peer, otherwise it happens on every cluster peer.
	Recover(ctx context.Context, ci cid.Cid, local bool) (*api.GlobalPinInfo, error)
	// RecoverAll triggers Recover() operations on all tracked items. If
	// local is true, the operation is limited to the current peer.
	// Otherwise, it happens everywhere.
	RecoverAll(ctx context.Context, local bool) ([]*api.GlobalPinInfo, error)

	// Version returns the ipfs-cluster peer's version.
	Version(context.Context) (*api.Version, error)

	// IPFS returns an instance of go-ipfs-api's Shell, pointing to a
	// Cluster's IPFS proxy endpoint.
	IPFS(context.Context) *httpapi.HttpApi

	// GetConnectGraph returns an ipfs-cluster connection graph.
	GetConnectGraph(context.Context) (*api.ConnectGraph, error)

	// Metrics returns a map with the latest metrics of matching name
	// for the current cluster peers.
	Metrics(ctx context.Context, name string) ([]*api.Metric, error)
}

package router

import (
	"context"

	lru "github.com/hashicorp/golang-lru"
	"github.com/ipfs/go-cid"
	"github.com/ipni/go-libipni/find/client"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/multiformats/go-multihash"
	gemipfs "github.com/willscott/go-gemipfs/lib"
)

// Router is trying to match requests to (by order of priority):
// 1. related request/response info in a retrieved car file
// 2. a previously made request archive in a known repo
// 3. a previously made request archive from a discovered repo
// 4. a new response made with a trusted relay
type Router struct {
	host    host.Host
	cache   *lru.ARCCache
	storage *gemipfs.CarStore
}

type RouterConfig struct {
	Store           *gemipfs.CarStore
	MemoryCacheSize int
}

func NewRouter(h host.Host, conf *RouterConfig) *Router {
	c, _ := lru.NewARC(conf.MemoryCacheSize)
	return &Router{
		host:    h,
		cache:   c,
		storage: conf.Store,
	}
}

func (r *Router) FindResponseInRepo(ctx context.Context, request cid.Cid, repo multiaddr.Multiaddr) (cid.Cid, error) {
	pid := peerIDFromMA(repo)
	stream, err := r.host.NewStream(ctx, pid, "/gemipfs/repo/0.0.1")
	if err != nil {
		return cid.Undef, err
	}
	stream.Write(request.Bytes())

	return cid.Undef, nil
}

// FindRepo helps with priority level 3
func (r *Router) FindRepos(ctx context.Context, domain cid.Cid) []multiaddr.Multiaddr {
	if val, ok := r.cache.Get(domain); ok {
		return val.([]multiaddr.Multiaddr)
	}

	cl, err := client.NewDHashClient(
		client.WithProvidersURL("https://cid.contact"),
		client.WithDHStoreURL("https://cid.contact"),
		client.WithPcacheTTL(0),
	)
	if err != nil {
		return []multiaddr.Multiaddr{}
	}

	mh := domain.Hash()
	resp, err := client.FindBatch(ctx, cl, []multihash.Multihash{mh})
	if err != nil {
		// todo: negative cache?
		return []multiaddr.Multiaddr{}
	}

	mar := make([]multiaddr.Multiaddr, 0, 1)
	for _, mhr := range resp.MultihashResults {
		if mhr.Multihash.String() != domain.Hash().String() {
			continue
		}
		for _, pr := range mhr.ProviderResults {
			// TODO: only add for providers supporting http?
			mar = append(mar, peerInfoToMAs(pr.Provider)...)
		}
	}
	r.cache.Add(domain, mar)

	return mar
}

func peerInfoToMAs(pi *peer.AddrInfo) []multiaddr.Multiaddr {
	out, err := peer.AddrInfoToP2pAddrs(pi)
	if err != nil {
		return []multiaddr.Multiaddr{}
	}
	return out
}

// Note: will return an invalid ID if this is not a multiaddr with a p2p component
func peerIDFromMA(m multiaddr.Multiaddr) peer.ID {
	ai, err := peer.AddrInfoFromP2pAddr(m)
	if err != nil {
		return peer.ID("")
	}
	return ai.ID
}

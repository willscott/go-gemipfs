package main

import (
	"context"
	"crypto/rand"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"path"
	"strconv"

	"github.com/elazarl/goproxy"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/sec"
	"github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"
	gemipfs "github.com/willscott/go-gemipfs/lib"
	"github.com/willscott/go-gemipfs/router"
)

func main() {
	verbose := flag.Bool("v", false, "should every proxy request be logged to stdout")
	addr := flag.String("addr", ":8080", "proxy listen address")
	resolverAddr := flag.String("remote", "127.0.0.1:8081", "where the resolver lives")
	repoAddr := flag.String("repo", "http://127.0.0.1:8082", "where the repo lives")
	storeLoc := flag.String("store", "./", "where to store data")
	flag.Parse()

	storeBaseLoc := path.Join(*storeLoc, ".gemipfs")
	store := gemipfs.NewCarStore(storeBaseLoc)
	rConf := router.RouterConfig{
		Store: store,
	}

	if err := getOrSetCA(); err != nil {
		log.Fatal(err)
		return
	}
	host, err := libp2p.New()
	if err != nil {
		log.Fatal(err)
		return
	}
	rh, rp, err := net.SplitHostPort(*resolverAddr)
	if err != nil {
		log.Fatalf("could not parse host %s: %v\n", *resolverAddr, err)
		return
	}
	peer, err := connectToPeer(context.Background(), host, rh, rp)
	if err != nil {
		log.Fatalf("could not connect: %v\n", err)
		return
	}
	repoUrl, err := url.Parse(*repoAddr)
	if err != nil {
		log.Fatalf("couldn't parse repo: %v\n", err)
		return
	}

	proxy := goproxy.NewProxyHttpServer()
	proxy.CertStore = NewCertStorage()
	proxy.OnRequest().HandleConnect(goproxy.AlwaysMitm)
	proxy.OnRequest().DoFunc(func(req *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
		gr := gemipfs.Wrap(req)
		decodedQuery, err := gr.Canonicalize().ToP2PQuery()
		if err != nil {
			log.Fatalf("could not serialize req to peer: %v\n", err)
			return nil, nil
		}
		contentSearchKey := decodedQuery.DomainHash()
		contentRouter := router.NewRouter(host, &rConf)

		// First, see if there's an existing repo with the content
		peers := contentRouter.FindRepos(req.Context(), contentSearchKey)
		storedResp, err := router.WithFirstToResolve(req.Context(), contentRouter, decodedQuery, peers)
		if err == nil {
			// return from an existing repo
			rb, err := store.Get(storedResp)
			if err != nil {
				log.Fatalf("could not retrieve response from store: %v\n", err)
				return nil, nil
			}
			gResp, err := gemipfs.ResponseFromWARC(req, rb)
			if err != nil {
				log.Fatalf("could not parse response from store: %v\n", err)
				return nil, nil
			}
			return req, gResp.HTTP(req)
		}

		// no store identified - use an exit to request the page.
		decodedQuery.Repo = repoUrl
		query, err := decodedQuery.EncryptTo(peer)
		if err != nil {
			log.Fatalf("could not serialize req to peer: %v\n", err)
			return nil, nil
		}
		netCtx, netCncl := context.WithCancel(req.Context())
		defer netCncl()
		stream, err := host.NewStream(netCtx, peer, "/exit/0.0.1")
		if err != nil {
			log.Fatal(err)
			return nil, nil
		}
		defer stream.Close()
		if err := query.Write(stream); err != nil {
			log.Fatal(err)
			return nil, nil
		}
		stream.CloseWrite()
		fmt.Printf("waiting for response for %s\n", req.URL)

		resp, err := gemipfs.ReadResponse(query.Resource, stream)
		if err != nil {
			log.Fatal(err)
			return nil, nil
		}
		return req, resp.HTTP(req)
	})
	proxy.Verbose = *verbose
	log.Fatal(http.ListenAndServe(*addr, proxy))
}

func connectToPeer(ctx context.Context, h host.Host, host string, port string) (peer.ID, error) {
	// make a synthetic id to connect to first
	_, fakePub, _ := crypto.GenerateEd25519Key(rand.Reader)
	fakeID, _ := peer.IDFromPublicKey(fakePub)
	hostAddr, err := netip.ParseAddr(host)
	if err != nil {
		return "", err
	}
	portInt, err := strconv.Atoi(port)
	if err != nil {
		return "", err
	}
	hostPortAddr := netip.AddrPortFrom(hostAddr, uint16(portInt))
	ma, err := manet.FromNetAddr(net.TCPAddrFromAddrPort(hostPortAddr))
	if err != nil {
		return "", err
	}
	err = h.Connect(ctx, peer.AddrInfo{ID: fakeID, Addrs: []multiaddr.Multiaddr{ma}})
	if err != nil {
		misMatchErr := sec.ErrPeerIDMismatch{}
		if !errors.As(err, &misMatchErr) {
			return "", err
		}
		err = h.Connect(ctx, peer.AddrInfo{ID: misMatchErr.Actual, Addrs: []multiaddr.Multiaddr{ma}})
		return misMatchErr.Actual, err
	}
	return "", nil
}

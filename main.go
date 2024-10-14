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
)

func main() {
	verbose := flag.Bool("v", false, "should every proxy request be logged to stdout")
	addr := flag.String("addr", ":8080", "proxy listen address")
	remote := flag.String("remote", "127.0.0.1:8081", "where the resolver lives")
	flag.Parse()
	if err := getOrSetCA(); err != nil {
		log.Fatal(err)
		return
	}
	host, err := libp2p.New()
	if err != nil {
		log.Fatal(err)
		return
	}
	rh, rp, err := net.SplitHostPort(*remote)
	if err != nil {
		log.Fatalf("could not parse host %s: %v\n", *remote, err)
		return
	}
	peer, err := connectToPeer(context.Background(), host, rh, rp)
	if err != nil {
		log.Fatalf("could not connect: %v\n", err)
		return
	}

	proxy := goproxy.NewProxyHttpServer()
	proxy.CertStore = NewCertStorage()
	proxy.OnRequest().HandleConnect(goproxy.AlwaysMitm)
	proxy.OnRequest().DoFunc(func(req *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
		gr := gemipfs.Wrap(req)
		query, err := gr.ToP2PQuery(peer)
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

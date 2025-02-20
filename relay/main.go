package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/network"
	manet "github.com/multiformats/go-multiaddr/net"
	gemipfs "github.com/willscott/go-gemipfs/lib"
)

func main() {
	addr := flag.String("addr", ":8080", "proxy listen address")
	flag.Parse()

	rh, rp, err := net.SplitHostPort(*addr)
	if err != nil {
		log.Fatalf("could not parse host %s: %v\n", *addr, err)
		return
	}
	hostAddr, err := netip.ParseAddr(rh)
	if err != nil {
		hostAddr = netip.MustParseAddr("0.0.0.0")
	}
	portInt, err := strconv.Atoi(rp)
	if err != nil {
		log.Fatalf("could not parse port %s: %v\n", *addr, err)
		return
	}
	hostPortAddr := netip.AddrPortFrom(hostAddr, uint16(portInt))
	ma, err := manet.FromNetAddr(net.TCPAddrFromAddrPort(hostPortAddr))
	if err != nil {
		log.Fatalf("could not parse host/port %s: %v\n", *addr, err)
		return
	}

	host, err := libp2p.New(libp2p.ListenAddrs(ma))
	if err != nil {
		log.Fatal(err)
		return
	}
	myID := gemipfs.Attester{
		Identity: host.Peerstore().PrivKey(host.ID()),
	}

	exitFunc := func(s network.Stream) {
		doExit(&myID, s)
	}
	host.SetStreamHandler("/exit/0.0.1", exitFunc)
	<-make(chan struct{})
}

func doExit(a *gemipfs.Attester, s network.Stream) {
	q, err := gemipfs.ReadQuery(s)
	if err != nil {
		log.Printf("could not read query: %v", err)
		return
	}
	dq, err := q.TryDecrypt(a.Identity)
	if err != nil {
		log.Printf("could not decrypt query: %v", err)
		return
	}

	ctx, cncl := context.WithTimeout(context.Background(), 10*time.Second)
	req, err := gemipfs.ParseRequest(ctx, dq.Request)
	if err != nil {
		cncl()
		log.Printf("could not read request: %v", err)
		return
	}
	fmt.Printf("going to req %s\n", req.URL)
	resp, err := req.Do(q.Resource, http.DefaultClient)
	if err != nil {
		cncl()
		log.Printf("could not fetch request: %v", err)
		return
	}
	cncl()
	fmt.Printf("finished request for %s\n", req.URL)
	prf, respBody := a.AttestResponse(resp)
	// push reponse to repo
	_, err = http.Post(dq.Repo.String(), "application/octet-stream", bytes.NewReader(respBody))
	if err != nil {
		log.Printf("failed to post to repo: %v", err)
		// failed to write to repo...
		return
	}

	// respond with attestation
	defer s.Close()
	_, err = s.Write(prf.Bytes())
	if err != nil {
		log.Printf("failed to write attestation: %v", err)
		return
	}
}

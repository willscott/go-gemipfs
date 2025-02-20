package router

import (
	"context"
	"errors"
	"sync"

	"github.com/ipfs/go-cid"
	"github.com/multiformats/go-multiaddr"
)

type QueryIface interface {
	Cid() cid.Cid
}

// TODO:
// 1. extend with a notion of 'reputation'
// 2. extend customization/flexibility around how many queries are made at once

func WithFirstToResolve(ctx context.Context, rtr *Router, query QueryIface, peers []multiaddr.Multiaddr) (cid.Cid, error) {
	if len(peers) == 0 {
		return cid.Undef, errors.New("no peers to query")
	}
	qCtx, cncl := context.WithCancel(ctx)
	wg := sync.WaitGroup{}
	dc := make(chan cid.Cid, 1)
	for _, p := range peers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := rtr.FindResponseInRepo(qCtx, query.Cid(), p)
			if err == nil {
				select {
				case dc <- resp:
					return
				default:
					return
				}
			}
		}()
	}
	go func() {
		wg.Wait()
		close(dc)
	}()
	out := <-dc
	cncl()
	return out, nil
}

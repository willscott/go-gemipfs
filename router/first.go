package router

import (
	"context"

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
	qCtx, cncl := context.WithCancel(ctx)
	dc := make(chan cid.Cid, 1)
	for _, p := range peers {
		go func() {
			resp, err := rtr.FindResponseInRepo(qCtx, query.Cid(), p)
			if err == nil {
				dc <- resp
			}
		}()
	}
	out := <-dc
	cncl()
	return out, nil
}

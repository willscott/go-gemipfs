package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	"github.com/ipld/go-car/v2/blockstore"
	"github.com/multiformats/go-multicodec"
	"github.com/multiformats/go-multihash"
)

type Repo struct {
	bs *blockstore.ReadWrite
}

func main() {
	storeLoc := flag.String("store", "tmp.car", "direct blockstore car")
	pubAddr := flag.String("pubaddr", ":8080", "public listen address")
	adminAddr := flag.String("adminaddr", ":8081", "admin listen address")
	flag.Parse()

	bsrw, err := blockstore.OpenReadWrite(*storeLoc, []cid.Cid{})
	if err != nil {
		fmt.Printf("couldn't open store: %v\n", err)
		return
	}
	defer bsrw.Close()

	R := Repo{
		bsrw,
	}

	pubHandler := http.NewServeMux()
	pubHandler.HandleFunc("/", R.repo)
	pubS := &http.Server{
		Addr:           *pubAddr,
		Handler:        pubHandler,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}

	adminHandler := http.NewServeMux()
	adminHandler.HandleFunc("/", issueToken)
	adminS := &http.Server{
		Addr:           *adminAddr,
		Handler:        adminHandler,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}
	go func() {
		pubS.ListenAndServe()
	}()
	go func() {
		adminS.ListenAndServe()
	}()
	<-make(chan struct{})

}

func (repo *Repo) repo(r http.ResponseWriter, req *http.Request) {
	if req.Method == "GET" {
		c := req.URL.Query().Get("cid")
		pc, err := cid.Parse(c)
		if err != nil {
			r.WriteHeader(406)
			r.Write([]byte("could not parse query"))
			return
		}
		blk, err := repo.bs.Get(req.Context(), pc)
		if err != nil {
			r.WriteHeader(404)
			return
		}
		log.Printf("get %s (resp is %d bytes)\n", blk.Cid().String(), len(blk.RawData()))
		r.WriteHeader(200)
		r.Write(blk.RawData())
	} else if req.Method == "POST" {
		blkb, err := io.ReadAll(req.Body)
		if err != nil {
			r.WriteHeader(406)
			return
		}
		v1b := cid.V1Builder{
			Codec:    uint64(multicodec.Https),
			MhType:   multihash.SHA2_256,
			MhLength: -1,
		}
		c1, _ := v1b.Sum(blkb)
		blk, _ := blocks.NewBlockWithCid(blkb, c1)
		repo.bs.Put(req.Context(), blk)
		r.WriteHeader(200)
		log.Printf("post %s\n", blk.Cid().String())
		r.Write(blk.Cid().Bytes())
	} else {
		r.WriteHeader(406)
		return
	}
}

func issueToken(r http.ResponseWriter, req *http.Request) {
	//TODO: privacy pass issuance/redemption
}

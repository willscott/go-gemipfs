package gemipfs

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"sync"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/hashicorp/golang-lru/v2/expirable"
	"github.com/ipfs/go-cid"
	car "github.com/ipld/go-car/v2"
	"github.com/ipld/go-car/v2/blockstore"
	"github.com/ipld/go-car/v2/index"
)

type CarStore struct {
	root      string
	maxFiles  int
	maxBlocks int

	entries     *expirable.LRU[string, *carEntry]
	lookupCache *lru.Cache[string, *carEntry]
}

type carEntry struct {
	file string
	idx  index.Index
	mtx  sync.RWMutex
}

func (ce *carEntry) Has(c cid.Cid) bool {
	ce.mtx.RLock()
	defer ce.mtx.RUnlock()
	if ce.idx == nil {
		return false
	}
	if o, e := index.GetFirst(ce.idx, c); e == nil && o > 0 {
		return true
	}
	return false
}

func (ce *carEntry) Get(c cid.Cid) ([]byte, error) {
	ce.mtx.RLock()
	defer ce.mtx.RUnlock()
	if ce.idx == nil {
		return nil, os.ErrNotExist
	}
	rdr, err := os.Open(ce.file)
	if err != nil {
		return nil, err
	}
	defer rdr.Close()
	bs, err := blockstore.NewReadOnly(rdr, ce.idx)
	if err != nil {
		return nil, err
	}
	blk, err := bs.Get(context.Background(), c)
	if err != nil {
		return nil, err
	}
	return blk.RawData(), nil
}

func (ce *carEntry) Cleanup() {
	ce.mtx.Lock()
	defer ce.mtx.Unlock()
	os.Remove(ce.file)
	ce.idx = nil
}

func NewCarStore(at string) *CarStore {
	cs := CarStore{
		at,
		1024,
		1024,
		nil,
		nil,
	}

	lookupCache, err := lru.New[string, *carEntry](1024)
	if err != nil {
		return nil
	}
	cs.lookupCache = lookupCache

	cs.entries = expirable.NewLRU[string, *carEntry](1024, cs.onEvict, time.Hour*24)

	return &cs
}

func (cs *CarStore) onEvict(k string, v *carEntry) {
	v.Cleanup()
}

func (c *CarStore) Add(archive io.ReadSeeker) error {
	idx, err := car.ReadOrGenerateIndex(archive)
	if err != nil {
		return err
	}
	archive.Seek(0, io.SeekStart)

	hdr, err := car.NewBlockReader(archive)
	if err != nil {
		return err
	}
	if len(hdr.Roots) != 1 {
		return car.ErrSizeMismatch
	}
	root := hdr.Roots[0]

	fileName := path.Join(c.root, fmt.Sprintf("%s.car", root))

	entry := carEntry{
		file: fileName,
		idx:  idx,
		mtx:  sync.RWMutex{},
	}
	entry.mtx.Lock()
	defer entry.mtx.Unlock()
	c.entries.Add(fileName, &entry)
	fp, err := os.OpenFile(fileName, os.O_CREATE, 0666)
	if err != nil {
		return err
	}
	defer fp.Close()
	io.Copy(fp, archive)
	return nil
}

func (c *CarStore) Get(itm cid.Cid) ([]byte, error) {
	if lce, ok := c.lookupCache.Get(itm.String()); ok {
		rdr, err := os.Open(lce.file)
		if err != nil {
			return nil, err
		}
		rbs, err := blockstore.NewReadOnly(rdr, lce.idx)
		if err == nil {
			if blk, err := rbs.Get(context.Background(), itm); err == nil {
				return blk.RawData(), nil
			}
		}
	}
	// slow path.
	files := c.entries.Values()
	for _, f := range files {
		if f.Has(itm) {
			// cache
			c.lookupCache.Add(itm.String(), f)
			return f.Get(itm)
		}
	}
	return nil, os.ErrNotExist
}

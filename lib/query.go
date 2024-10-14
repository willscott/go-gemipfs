package gemipfs

import (
	"bytes"
	"io"

	"filippo.io/age"
	"github.com/ipfs/go-cid"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	mc "github.com/multiformats/go-multicodec"
	"github.com/multiformats/go-multihash"
	cbor "github.com/whyrusleeping/cbor/go"
	agep2p "github.com/willscott/go-gemipfs/age"
)

type Query struct {
	Resource     cid.Cid
	QueryContext []byte
}

func (q *Query) TryDecrypt(id crypto.PrivKey) (*DecodedQuery, error) {
	ident := agep2p.NewLibP2PIdentity(id)
	buf := bytes.NewBuffer(q.QueryContext)
	out, err := age.Decrypt(buf, ident)
	if err != nil {
		return nil, err
	}
	dcoder := cbor.NewDecoder(out)
	sr := req{}
	if err := dcoder.Decode(&sr); err != nil {
		return nil, err
	}
	return &DecodedQuery{
		Resource:          q.Resource,
		serializedRequest: sr,
	}, nil
}

func (q *Query) Write(w io.Writer) error {
	if _, err := w.Write(q.Resource.Bytes()); err != nil {
		return err
	}
	if _, err := w.Write(q.QueryContext); err != nil {
		return err
	}
	return nil
}

func ReadQuery(r io.Reader) (*Query, error) {
	_, rsrc, err := cid.CidFromReader(r)
	if err != nil {
		return nil, err
	}
	ctxt, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return &Query{
		Resource:     rsrc,
		QueryContext: ctxt,
	}, nil

}

func (dq *DecodedQuery) EncryptTo(p peer.ID) (*Query, error) {
	lr := agep2p.NewLibP2PRecipient(p)
	out := bytes.NewBuffer(nil)
	stream, err := age.Encrypt(out, lr)
	if err != nil {
		return nil, err
	}
	if err := cbor.Encode(stream, dq.serializedRequest); err != nil {
		return nil, err
	}
	stream.Close()

	// the query is for a derived hash.
	mh, _ := multihash.Sum(dq.Resource.Bytes(), multihash.SHA2_256, -1)
	wireResource := cid.NewCidV1(uint64(mc.Https), mh)

	return &Query{
		Resource:     wireResource,
		QueryContext: out.Bytes(),
	}, nil
}

type DecodedQuery struct {
	Resource          cid.Cid
	serializedRequest req
}

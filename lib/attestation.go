package gemipfs

import (
	"github.com/ipfs/go-cid"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/polydawn/refmt/cbor"
)

type Attester struct {
	Identity crypto.PrivKey
}

type Attestation struct {
	Req  cid.Cid
	Resp cid.Cid
	Sig  []byte
}

func (a *Attester) AttestResponse(r *Response) (*Attestation, []byte) {
	rCid, rBody := r.Serialize()
	s, _ := a.Identity.Sign(append(r.Query.Cid.Bytes(), rCid.Bytes()...))

	return &Attestation{
		Req:  r.Query.Cid,
		Resp: rCid,
		Sig:  s,
	}, rBody
}

func (a *Attestation) Bytes() []byte {
	b, err := cbor.Marshal(a)
	if err != nil {
		return []byte{}
	}
	return b
}

func ParseAttestation(b []byte) *Attestation {
	a := Attestation{}
	_ = cbor.Unmarshal(cbor.DecodeOptions{}, b, &a)
	return &a
}

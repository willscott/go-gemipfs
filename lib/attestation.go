package gemipfs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"

	"github.com/ipfs/go-cid"
	"github.com/libp2p/go-libp2p/core/crypto"
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
	s, _ := a.Identity.Sign(append(r.Query.Bytes(), rCid.Bytes()...))
	fmt.Printf("attesting %s -> %s\n", r.Query, rCid)

	return &Attestation{
		Req:  r.Query,
		Resp: rCid,
		Sig:  s,
	}, rBody
}

func (a *Attestation) Bytes() []byte {
	buf := bytes.NewBuffer(nil)
	err := json.NewEncoder(buf).Encode(a)
	if err != nil {
		log.Printf("failed to marshal attestation: %v", err)
		return []byte{}
	}
	return buf.Bytes()
}

func ParseAttestation(b []byte) (*Attestation, error) {
	a := Attestation{}
	err := json.NewDecoder(bytes.NewReader(b)).Decode(&a)
	return &a, err
}

package agep2p

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"fmt"

	"filippo.io/age"
	"filippo.io/age/agessh"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
)

func NewLibP2PIdentity(pk crypto.PrivKey) *LibP2PIdentity {
	return &LibP2PIdentity{pk}
}

type LibP2PIdentity struct {
	myPrivKey crypto.PrivKey
}

func (li *LibP2PIdentity) Recipient() *LibP2PRecipient {
	pid, err := peer.IDFromPrivateKey(li.myPrivKey)
	if err != nil {
		return nil
	}
	return NewLibP2PRecipient(pid)
}

func (li *LibP2PIdentity) Unwrap(stanzas []*age.Stanza) ([]byte, error) {
	actualKey, err := crypto.PrivKeyToStdKey(li.myPrivKey)
	if err != nil {
		return nil, err
	}

	switch p := actualKey.(type) {
	case *rsa.PrivateKey:
		sshp, err := agessh.NewRSAIdentity(p)
		if err != nil {
			return nil, err
		}
		return sshp.Unwrap(stanzas)

	case *ecdsa.PrivateKey:
		return nil, fmt.Errorf("TODO: ecdsa not currently supported")

	case *ed25519.PrivateKey:
		sshp, err := agessh.NewEd25519Identity(*p)
		if err != nil {
			return nil, err
		}
		return sshp.Unwrap(stanzas)

	case *secp256k1.PrivateKey:
		return nil, fmt.Errorf("TODO: secp256k1 not currently supported")

	default:
		return nil, fmt.Errorf("unsupported key type")
	}
}

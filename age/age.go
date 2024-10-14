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
	"golang.org/x/crypto/ssh"
)

func NewLibP2PRecipient(p peer.ID) *LibP2PRecipient {
	return &LibP2PRecipient{p}
}

type LibP2PRecipient struct {
	theirPubKey peer.ID
}

func (r *LibP2PRecipient) Wrap(fileKey []byte) ([]*age.Stanza, error) {
	actualLibP2PKey, err := r.theirPubKey.ExtractPublicKey()
	if err != nil {
		return nil, err
	}
	actualKey, err := crypto.PubKeyToStdKey(actualLibP2PKey)
	if err != nil {
		return nil, err
	}

	switch p := actualKey.(type) {
	case *rsa.PublicKey, rsa.PublicKey:
		sshp, err := ssh.NewPublicKey(p)
		if err != nil {
			return nil, err
		}
		ageR, err := agessh.NewRSARecipient(sshp)
		if err != nil {
			return nil, err
		}
		return ageR.Wrap(fileKey)

	case *ecdsa.PublicKey:
		return nil, fmt.Errorf("TODO: ecdsa not currently supported")

	case *ed25519.PublicKey, ed25519.PublicKey:
		sshp, err := ssh.NewPublicKey(p)
		if err != nil {
			return nil, err
		}
		ageR, err := agessh.NewEd25519Recipient(sshp)
		if err != nil {
			return nil, err
		}
		return ageR.Wrap(fileKey)

	case *secp256k1.PublicKey:
		return nil, fmt.Errorf("TODO: secp256k1 not currently supported")

	default:
		return nil, fmt.Errorf("unsupported key type: %T", p)
	}
}

// String returns the Bech32 public key encoding of r.
func (r *LibP2PRecipient) String() string {
	return r.theirPubKey.String()
}

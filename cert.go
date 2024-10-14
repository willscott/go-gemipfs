package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"os"
	"time"

	"github.com/elazarl/goproxy"
)

func getOrSetCA() error {
	var ca []byte
	var pk []byte
	var err error

	if _, err := os.Stat("cert.pem"); os.IsNotExist(err) {
		// make
		priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return err
		}
		serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
		serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
		if err != nil {
			return errors.New("failed to generate serial number: " + err.Error())
		}

		tmpl := &x509.Certificate{
			IsCA:                  true,
			SerialNumber:          serialNumber,
			Subject:               pkix.Name{Organization: []string{"gemipfs"}},
			SignatureAlgorithm:    x509.ECDSAWithSHA256,
			NotBefore:             time.Now(),
			NotAfter:              time.Now().Add(time.Hour * 24 * 30), // valid for a month
			BasicConstraintsValid: true,
		}
		tmpl.KeyUsage = x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature
		tmpl.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}
		publicKey := &priv.PublicKey
		caDer, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, publicKey, priv)
		if err != nil {
			return err
		}

		pkb, err := x509.MarshalECPrivateKey(priv)
		if err != nil {
			return err
		}
		pk = pem.EncodeToMemory(&pem.Block{
			Type: "ECDSA PRIVATE KEY", Bytes: pkb,
		})

		if err = os.WriteFile("priv.pem", pk, 0644); err != nil {
			return err
		}

		b := pem.Block{Type: "CERTIFICATE", Bytes: caDer}
		ca = pem.EncodeToMemory(&b)
		if err = os.WriteFile("cert.pem", ca, 0644); err != nil {
			return err
		}
	} else {
		if ca, err = os.ReadFile("cert.pem"); err != nil {
			return err
		}
		if pk, err = os.ReadFile("priv.pem"); err != nil {
			return err
		}
	}
	goproxyCa, err := tls.X509KeyPair(ca, pk)
	if err != nil {
		return err
	}

	goproxy.GoproxyCa = goproxyCa
	goproxy.OkConnect = &goproxy.ConnectAction{Action: goproxy.ConnectAccept, TLSConfig: goproxy.TLSConfigFromCA(&goproxyCa)}
	goproxy.MitmConnect = &goproxy.ConnectAction{Action: goproxy.ConnectMitm, TLSConfig: goproxy.TLSConfigFromCA(&goproxyCa)}
	goproxy.HTTPMitmConnect = &goproxy.ConnectAction{Action: goproxy.ConnectHTTPMitm, TLSConfig: goproxy.TLSConfigFromCA(&goproxyCa)}
	goproxy.RejectConnect = &goproxy.ConnectAction{Action: goproxy.ConnectReject, TLSConfig: goproxy.TLSConfigFromCA(&goproxyCa)}
	return nil
}

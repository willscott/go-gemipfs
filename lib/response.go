package gemipfs

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"os"
	"time"

	"github.com/CorentinB/warc"
	"github.com/ipfs/go-cid"
	mc "github.com/multiformats/go-multicodec"
	"github.com/multiformats/go-multihash"
	cbor "github.com/whyrusleeping/cbor/go"
	"golang.org/x/crypto/nacl/box"
)

type Response struct {
	Query      cid.Cid
	req        *Request
	Transcript []byte
}

func (r *Response) Write(w io.Writer) error {
	buf := bytes.NewBuffer(nil)
	if err := cbor.Encode(buf, r); err != nil {
		return err
	}
	sk := sha256.New().Sum(r.Query.Hash())
	skf := (*[32]byte)(sk)
	nonce := sha256.New().Sum(append([]byte("nonce"), r.Query.Hash()...))
	noncef := (*[24]byte)(nonce)
	enc := box.SealAfterPrecomputation([]byte{}, buf.Bytes(), noncef, skf)
	_, err := w.Write(enc)
	return err
}

func (r *Response) Expiry() time.Duration {
	//todo: get from headers
	return 5 * time.Minute
}

func (r *Response) Serialize() (cid.Cid, []byte) {
	sk := sha256.New().Sum(r.Query.Hash())
	skf := (*[32]byte)(sk)
	nonce := sha256.New().Sum(append([]byte("nonce"), r.Query.Hash()...))
	noncef := (*[24]byte)(nonce)
	enc := box.SealAfterPrecomputation([]byte{}, r.Transcript, noncef, skf)

	mh, _ := multihash.Sum(enc, multihash.SHA2_256, -1)
	c := cid.NewCidV1(uint64(mc.Https), mh)

	fmt.Printf("sealed %s with shared key %+x", c, skf)

	return c, enc
}

func ReadResponse(query cid.Cid, r io.Reader) (*Response, error) {
	sk := sha256.New().Sum(query.Hash())
	skf := (*[32]byte)(sk)
	nonce := sha256.New().Sum(append([]byte("nonce"), query.Hash()...))
	noncef := (*[24]byte)(nonce)

	buf, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	transcript, ok := box.OpenAfterPrecomputation([]byte{}, buf, noncef, skf)
	if !ok {
		return nil, fmt.Errorf("failed to decrypt %s usking shared key %+x", query, skf)
	}

	rsp := Response{}
	rsp.Query = query
	rsp.Transcript = transcript
	return &rsp, nil
}

func (r *Response) HTTP(req *http.Request) (*http.Response, error) {
	ncr := io.NopCloser(bytes.NewReader(r.Transcript))
	reader, err := warc.NewReader(ncr)
	if err != nil {
		return nil, err
	}
	rcrd, _, err := reader.ReadRecord()
	if err != nil {
		return nil, err
	}

	return http.ReadResponse(bufio.NewReader(rcrd.Content), req)
}

func ResponseFromWARC(q cid.Cid, httpReq *http.Request, respArc []byte) (*Response, error) {
	req, err := Wrap(httpReq)
	if err != nil {
		return nil, err
	}
	return &Response{
		Query:      q,
		req:        req,
		Transcript: respArc,
	}, nil
}

func ResponseFrom(q cid.Cid, r *Request, hr *http.Response) (*Response, error) {
	dumpResponse, err := httputil.DumpResponse(hr, true)
	if err != nil {
		return nil, err
	}

	rw := bytes.NewReader(dumpResponse)
	digest := "sha1:" + warc.GetSHA1(rw)
	respArc := warc.NewRecord(os.TempDir(), false)
	respArc.Header.Set("WARC-Type", "response")
	respArc.Header.Set("WARC-Payload-Digest", digest)
	respArc.Header.Set("WARC-Block-Digest", digest)
	respArc.Header.Set("WARC-Target-URI", r.URL.String())
	respArc.Header.Set("WARC-Date", r.Time.UTC().Format(time.RFC3339Nano))
	respArc.Header.Set("WARC-Record-ID", "<urn:uuid:"+r.UUID.String()+">")
	respArc.Header.Set("Host", r.URL.Host)
	respArc.Header.Set("Content-Type", "application/http; msgtype=response")
	respArc.Content.Write(dumpResponse)

	buf := bytes.NewBuffer(nil)
	writer := &warc.Writer{
		FileName:    "",
		Compression: "",
		FileWriter:  bufio.NewWriter(buf),
	}
	_, err = writer.WriteRecord(respArc)
	if err != nil {
		return nil, err
	}

	return &Response{
		Query:      q,
		req:        r,
		Transcript: buf.Bytes(),
	}, nil
}

package gemipfs

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/ipfs/go-cid"
	cbor "github.com/whyrusleeping/cbor/go"
	"golang.org/x/crypto/nacl/box"
)

type Response struct {
	Query      cid.Cid
	Status     string
	StatusCode int
	Headers    []string
	Body       []byte
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

func ReadResponse(query cid.Cid, r io.Reader) (*Response, error) {
	sk := sha256.New().Sum(query.Hash())
	skf := (*[32]byte)(sk)
	nonce := sha256.New().Sum(append([]byte("nonce"), query.Hash()...))
	noncef := (*[24]byte)(nonce)

	buf, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	stream, ok := box.OpenAfterPrecomputation([]byte{}, buf, noncef, skf)
	if !ok {
		return nil, fmt.Errorf("failed to decrypt")
	}

	d := cbor.NewDecoder(bytes.NewReader(stream))
	rsp := Response{}
	if err := d.Decode(&rsp); err != nil {
		return nil, err
	}
	return &rsp, nil
}

func (r *Response) HTTP(req *http.Request) *http.Response {
	hr := http.Response{
		Status:     r.Status,
		StatusCode: r.StatusCode,
		Header:     make(http.Header),
		Body:       &bufRC{bytes.NewReader(r.Body)},
		Request:    req,
	}
	for _, h := range r.Headers {
		kv := strings.SplitN(h, ":", 2)
		hr.Header.Add(kv[0], kv[1])
	}
	return &hr
}

func ResponseFrom(q cid.Cid, hr *http.Response) *Response {
	var headerLines []string
	body, _ := io.ReadAll(hr.Body)

	for k, v := range hr.Header {
		for _, vv := range v {
			headerLines = append(headerLines, fmt.Sprintf("%s:%s", k, vv))
		}
	}

	return &Response{
		Query:      q,
		Status:     hr.Status,
		StatusCode: hr.StatusCode,
		Headers:    headerLines,
		Body:       body,
	}
}

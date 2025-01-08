package gemipfs

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"os"
	"strings"

	"github.com/CorentinB/warc"
	"github.com/ipfs/go-cid"
	mc "github.com/multiformats/go-multicodec"
	"github.com/multiformats/go-multihash"
	cbor "github.com/whyrusleeping/cbor/go"
)

type Request struct {
	cid.Cid
	*http.Request
}

func Wrap(r *http.Request) *Request {
	return &Request{
		cid.Undef,
		r,
	}
}

// serializable request
type req struct {
	Method  string
	URL     string
	Headers []string
	Body    []byte
}

type bufRC struct{ *bytes.Reader }

func (brc *bufRC) Close() error {
	return nil
}
func (brc *bufRC) Read(b []byte) (int, error) {
	return brc.Reader.Read(b)
}

func (r *Request) toSerial() *req {
	var headerLines []string
	body, _ := io.ReadAll(r.Request.Body)
	brc := bufRC{bytes.NewReader(body)}
	r.Request.Body = &brc

	for k, v := range r.Request.Header {
		for _, vv := range v {
			headerLines = append(headerLines, fmt.Sprintf("%s:%s", k, vv))
		}
	}
	re := req{
		Method:  r.Request.Method,
		URL:     r.Request.URL.String(),
		Headers: headerLines,
		Body:    body,
	}
	return &re
}

func (r *Request) Hash() cid.Cid {
	if !r.Cid.Defined() {
		buf := bytes.NewBuffer(nil)
		cbor.Encode(buf, r.toSerial())
		mh, _ := multihash.Sum(buf.Bytes(), multihash.SHA2_256, -1)
		r.Cid = cid.NewCidV1(uint64(mc.Https), mh)
	}
	return r.Cid
}

// Canonicalize performs available transformations to try to make it more likely
// that subequent requests for "the same" content result in the same queries.
func (r *Request) Canonicalize() *Request {
	return r
}

func (r *Request) ToP2PQuery() (*DecodedQuery, error) {
	sr := r.toSerial()
	if !r.Cid.Defined() {
		buf := bytes.NewBuffer(nil)
		cbor.Encode(buf, sr)
		mh, _ := multihash.Sum(buf.Bytes(), multihash.SHA2_256, -1)
		r.Cid = cid.NewCidV1(uint64(mc.Https), mh)
	}

	return &DecodedQuery{
		Resource:          r.Cid,
		serializedRequest: *sr,
	}, nil
}

func ParseRequest(ctx context.Context, q *DecodedQuery) *Request {
	hr, err := http.NewRequestWithContext(ctx, q.serializedRequest.Method, q.serializedRequest.URL, bytes.NewReader(q.serializedRequest.Body))
	for _, h := range q.serializedRequest.Headers {
		kv := strings.SplitN(h, ":", 2)
		hr.Header.Add(kv[0], kv[1])
	}
	if err != nil {
		return nil
	}
	return &Request{
		q.Resource,
		hr,
	}
}

func (r *Request) Do(c http.Client) (*Response, error) {
	dumpRequest, err := httputil.DumpRequest(r.Request, true)
	if err != nil {
		return nil, err
	}
	rw := bytes.NewReader(dumpRequest)
	reqArc := warc.NewRecord(os.TempDir(), false)
	reqArc.Header.Set("WARC-Type", "request")
	reqArc.Header.Set("WARC-Payload-Digest", "sha1:"+warc.GetSHA1(rw))
	reqArc.Header.Set("WARC-Target-URI", r.URL.String())
	reqArc.Header.Set("Host", r.URL.Host)
	reqArc.Header.Set("Content-Type", "application/http; msgtype=request")
	reqArc.Content.Write(dumpRequest)

	hr, err := c.Do(r.Request)
	if err != nil {
		return nil, err
	}

	dumpResponse, err := httputil.DumpResponse(hr, true)
	if err != nil {
		return nil, err
	}
	drR := bytes.NewReader(dumpResponse)

	// Add the response to the exchange
	var respArc = warc.NewRecord(os.TempDir(), false)
	respArc.Header.Set("WARC-Type", "response")
	respArc.Header.Set("WARC-Payload-Digest", "sha1:"+warc.GetSHA1(drR))
	respArc.Header.Set("WARC-Target-URI", hr.Request.URL.String())
	respArc.Header.Set("Content-Type", "application/http; msgtype=response")
	respArc.Content.Write(dumpResponse)

	return ResponseFrom(r, hr), nil
}

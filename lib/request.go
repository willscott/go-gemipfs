package gemipfs

import (
	"bufio"
	"bytes"
	"context"
	"net/http"
	"net/http/httputil"
	"os"
	"strings"
	"time"

	"github.com/CorentinB/warc"
	"github.com/google/uuid"
	"github.com/ipfs/go-cid"
	mc "github.com/multiformats/go-multicodec"
	"github.com/multiformats/go-multihash"
	cbor "github.com/whyrusleeping/cbor/go"
)

type Request struct {
	time.Time
	uuid.UUID
	*http.Request
}

func Wrap(hr *http.Request) (*Request, error) {
	r := &Request{
		time.Now(),
		uuid.New(),
		hr,
	}
	return r, nil
}

// serializable request
type SerializedRequest []byte

type bufRC struct{ *bytes.Reader }

func (brc *bufRC) Close() error {
	return nil
}
func (brc *bufRC) Read(b []byte) (int, error) {
	return brc.Reader.Read(b)
}

func (r *Request) Serialize() (SerializedRequest, error) {
	dumpRequest, err := httputil.DumpRequest(r.Request, true)
	if err != nil {
		return nil, err
	}
	rw := bytes.NewReader(dumpRequest)
	digest := "sha1:" + warc.GetSHA1(rw)
	reqArc := warc.NewRecord(os.TempDir(), false)
	reqArc.Header.Set("WARC-Type", "request")
	reqArc.Header.Set("WARC-Payload-Digest", digest)
	reqArc.Header.Set("WARC-Block-Digest", digest)
	reqArc.Header.Set("WARC-Target-URI", r.URL.String())
	reqArc.Header.Set("WARC-Date", r.Time.UTC().Format(time.RFC3339Nano))
	reqArc.Header.Set("WARC-Record-ID", "<urn:uuid:"+r.UUID.String()+">")
	reqArc.Header.Set("Host", r.URL.Host)
	reqArc.Header.Set("Content-Type", "application/http; msgtype=request")
	reqArc.Content.Write(dumpRequest)

	buf := bytes.NewBuffer(nil)
	writer := &warc.Writer{
		FileName:    "",
		Compression: "",
		FileWriter:  bufio.NewWriter(buf),
	}
	_, err = writer.WriteRecord(reqArc)
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// Canonicalize performs available transformations to try to make it more likely
// that subequent requests for "the same" content result in the same queries.
func (r *Request) Canonicalize() *Request {
	// TODO: date quantized based on expected etag / etc
	// TODO: strip un-needed browser UA / other headers that shouldn't change response
	return r
}

func ParseRequest(ctx context.Context, sr SerializedRequest) (*Request, error) {
	brc := bufRC{bytes.NewReader(sr)}
	reader, err := warc.NewReader(&brc)
	if err != nil {
		return nil, err
	}
	rcrd, _, err := reader.ReadRecord()
	if err != nil {
		return nil, err
	}

	reqTmpl, err := http.ReadRequest(bufio.NewReader(rcrd.Content))
	if err != nil {
		return nil, err
	}
	hr, err := http.NewRequestWithContext(ctx, reqTmpl.Method, reqTmpl.URL.String(), reqTmpl.Body)
	if err != nil {
		return nil, err
	}

	dt := time.Now()
	date := rcrd.Header.Get("WARC-Date")
	if date != "" {
		dt, err = time.Parse(time.RFC3339Nano, date)
		if err != nil {
			return nil, err
		}
	}

	uuidStr := rcrd.Header.Get("WARC-Record-ID")
	uuidStr, _ = strings.CutPrefix(uuidStr, "<urn:uuid:")
	uuidStr, _ = strings.CutSuffix(uuidStr, ">")
	uuid, err := uuid.Parse(uuidStr)
	if err != nil {
		return nil, err
	}

	return &Request{
		Time:    dt,
		UUID:    uuid,
		Request: hr,
	}, nil
}

func (r *Request) Do(q cid.Cid, c *http.Client) (*Response, error) {
	hr, err := c.Do(r.Request)
	if err != nil {
		return nil, err
	}

	return ResponseFrom(q, r, hr)
}

func (r *Request) DomainHash() cid.Cid {
	// TODO: better fingerprint
	base := r.URL.Scheme + "://" + r.URL.Host + "/"

	buf := bytes.NewBuffer(nil)
	cbor.Encode(buf, base)
	mh, _ := multihash.Sum(buf.Bytes(), multihash.SHA2_256, -1)
	return cid.NewCidV1(uint64(mc.Https), mh)
}

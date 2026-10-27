package httputil

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

var (
	ErrFailedToCreateRequest    = errors.New("httputil: failed to create request")
	ErrFailedToSendRequest      = errors.New("httputil: failed to send request")
	ErrFailedToReadResponseBody = errors.New("httputil: failed to read response body")
	ErrUnexpectedStatusCode     = errors.New("httputil: unexpected status code")
	ErrResponseBodyTooLarge     = errors.New("httputil: response body too large")
)

const (
	// requestTimeout bounds one request on top of ctx, so a stalled server
	// cannot hang a caller whose ctx has no deadline.
	requestTimeout = 30 * time.Second

	// maxErrorBodyBytes bounds how much of a non-2xx response body goes into
	// the error message.
	maxErrorBodyBytes = 512

	// maxResponseBodyBytes bounds how much of a 2xx response body is read, so a
	// misbehaving server cannot exhaust memory. A Jira search page of 100 issues
	// with ADF descriptions is a few MiB at most; 32 MiB leaves ample headroom.
	maxResponseBodyBytes = 32 << 20
)

// HTTPClient sends HTTP requests and returns the response body of 2xx
// responses. Any other status is an ErrUnexpectedStatusCode error.
type HTTPClient interface {
	Get(ctx context.Context, url string, header http.Header) ([]byte, error)
	Post(ctx context.Context, url string, header http.Header, body []byte) ([]byte, error)
}

type httpClient struct {
	client *http.Client
}

// NewHTTPClient returns an HTTPClient backed by net/http with a fixed request timeout.
func NewHTTPClient() HTTPClient {
	return &httpClient{client: &http.Client{Timeout: requestTimeout}}
}

// Get sends a GET request to url with header and returns the response body.
func (c *httpClient) Get(ctx context.Context, url string, header http.Header) ([]byte, error) {
	return c.do(ctx, http.MethodGet, url, header, nil)
}

// Post sends a POST request with body to url with header and returns the response body.
func (c *httpClient) Post(ctx context.Context, url string, header http.Header, body []byte) ([]byte, error) {
	return c.do(ctx, http.MethodPost, url, header, bytes.NewReader(body))
}

// do never puts header values or URL userinfo into errors, since they carry
// credentials.
func (c *httpClient) do(
	ctx context.Context,
	method, rawURL string,
	header http.Header,
	body io.Reader,
) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, rawURL, body)
	if err != nil {
		// A *url.Error quotes the raw URL; keep only its cause.
		if urlErr, ok := errors.AsType[*url.Error](err); ok {
			err = urlErr.Err
		}

		return nil, errors.Join(fmt.Errorf("%w: method=%s", ErrFailedToCreateRequest, method), err)
	}

	if header != nil {
		req.Header = header.Clone()
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("%w: method=%s", ErrFailedToSendRequest, method), err)
	}

	// Only the bounded prefix of the body is used, so a close error loses nothing.
	defer func() { _ = resp.Body.Close() }()

	ok := resp.StatusCode >= 200 && resp.StatusCode <= 299

	limit := int64(maxResponseBodyBytes)
	if !ok {
		limit = maxErrorBodyBytes
	}

	// One byte past the limit tells an oversized body from one exactly at it.
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, errors.Join(
			fmt.Errorf(
				"%w: method=%s, url=%s, status=%d",
				ErrFailedToReadResponseBody,
				method,
				withoutUserinfo(req.URL),
				resp.StatusCode,
			),
			err,
		)
	}

	if !ok {
		return nil, fmt.Errorf(
			"%w: method=%s, url=%s, status=%d, body=%s",
			ErrUnexpectedStatusCode,
			method,
			withoutUserinfo(req.URL),
			resp.StatusCode,
			data[:min(len(data), maxErrorBodyBytes)],
		)
	}

	if len(data) > maxResponseBodyBytes {
		return nil, fmt.Errorf(
			"%w: method=%s, url=%s, limit=%d",
			ErrResponseBodyTooLarge,
			method,
			withoutUserinfo(req.URL),
			maxResponseBodyBytes,
		)
	}

	return data, nil
}

// withoutUserinfo formats u with its userinfo dropped, since url.URL.Redacted
// masks only the password and would echo a username-only secret.
func withoutUserinfo(u *url.URL) string {
	c := *u
	c.User = nil

	return c.String()
}

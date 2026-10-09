package proxy

import (
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/klauspost/compress/zstd"
)

const supportedContentEncodings = "gzip, zstd"

var errUnsupportedContentEncoding = errors.New("unsupported content encoding")

// decodedBodyReader decodes Content-Encoding in reverse application order. The
// caller must close both the decoders and the original body, including on errors.
func decodedBodyReader(body io.Reader, headers http.Header) (io.Reader, func(), error) {
	encodings := strings.Split(strings.Join(headers.Values("Content-Encoding"), ","), ",")
	for i, encoding := range encodings {
		encodings[i] = strings.ToLower(strings.TrimSpace(encoding))
		switch encodings[i] {
		case "", "identity", "gzip", "x-gzip", "zstd":
		default:
			return nil, func() {}, fmt.Errorf("%w: %s", errUnsupportedContentEncoding, encoding)
		}
	}

	var decoders []io.Closer
	closeDecoders := func() {
		for i := len(decoders) - 1; i >= 0; i-- {
			_ = decoders[i].Close()
		}
	}
	reader := body
	for i := len(encodings) - 1; i >= 0; i-- {
		var decoder io.ReadCloser
		var err error
		switch encodings[i] {
		case "gzip", "x-gzip":
			decoder, err = gzip.NewReader(reader)
		case "zstd":
			// Synchronous decoding keeps SSE output incremental without reading
			// ahead for additional blocks before returning the current one.
			var zstdReader *zstd.Decoder
			zstdReader, err = zstd.NewReader(reader, zstd.WithDecoderConcurrency(1))
			if err == nil {
				decoder = zstdReader.IOReadCloser()
			}
		default:
			continue
		}
		if err != nil {
			closeDecoders()
			return nil, func() {}, fmt.Errorf("decode %s body: %w", encodings[i], err)
		}
		decoders = append(decoders, decoder)
		reader = decoder
	}
	return reader, closeDecoders, nil
}

func decodedResponseBodyReader(resp *http.Response) (io.Reader, func(), error) {
	// HEAD, 204 and 304 responses may describe an encoded representation while
	// carrying no body. There is no compressed payload to decode in that case.
	if resp.Body == http.NoBody || resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusNotModified ||
		(resp.Request != nil && resp.Request.Method == http.MethodHead) {
		return resp.Body, func() {}, nil
	}
	reader, closeDecoders, err := decodedBodyReader(resp.Body, resp.Header)
	if err == nil {
		clearBodyEncodingHeaders(resp.Header)
		resp.ContentLength = -1
	}
	return reader, closeDecoders, err
}

func clearBodyEncodingHeaders(headers http.Header) {
	headers.Del("Content-Encoding")
	headers.Del("Content-Length")
}

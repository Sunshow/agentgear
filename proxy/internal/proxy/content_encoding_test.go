package proxy

import (
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"
)

func encodeTestBody(t *testing.T, body []byte, encoding string) []byte {
	t.Helper()
	for _, coding := range strings.Split(encoding, ",") {
		var compressed bytes.Buffer
		var writer io.WriteCloser
		switch strings.ToLower(strings.TrimSpace(coding)) {
		case "", "identity":
			continue
		case "gzip", "x-gzip":
			writer = gzip.NewWriter(&compressed)
		case "zstd":
			var err error
			writer, err = zstd.NewWriter(&compressed, zstd.WithEncoderConcurrency(1))
			if err != nil {
				t.Fatal(err)
			}
		default:
			t.Fatalf("unsupported test encoding %q", coding)
		}
		if _, err := writer.Write(body); err != nil {
			t.Fatal(err)
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		body = compressed.Bytes()
	}
	return body
}

func TestDecodedBodyReader(t *testing.T) {
	body := []byte(`{"messages":[{"role":"user","content":"你好，zstd"}],"stream":true}`)
	for _, encoding := range []string{"", "identity", "gzip", "zstd", " GZip ", " ZSTD ", "x-gzip", "gzip, zstd", "zstd, gzip"} {
		t.Run(encoding, func(t *testing.T) {
			compressed := encodeTestBody(t, body, encoding)
			// Multiple header values are equivalent to a comma-separated list.
			headers := http.Header{"Content-Encoding": strings.Split(encoding, ",")}
			reader, closeDecoders, err := decodedBodyReader(bytes.NewReader(compressed), headers)
			if err != nil {
				t.Fatal(err)
			}
			defer closeDecoders()
			decoded, err := io.ReadAll(reader)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(decoded, body) {
				t.Fatalf("decoded body = %q, want %q", decoded, body)
			}
		})
	}
}

func TestDecodedBodyReaderRejectsInvalidData(t *testing.T) {
	for _, encoding := range []string{"gzip", "zstd"} {
		t.Run(encoding, func(t *testing.T) {
			valid := encodeTestBody(t, []byte(`{"ok":true}`), encoding)
			for name, body := range map[string][]byte{
				"corrupt":   []byte("invalid compressed body"),
				"truncated": valid[:len(valid)-3],
			} {
				t.Run(name, func(t *testing.T) {
					reader, closeDecoders, err := decodedBodyReader(bytes.NewReader(body), http.Header{"Content-Encoding": {encoding}})
					defer closeDecoders()
					if err == nil {
						_, err = io.ReadAll(reader)
					}
					if err == nil {
						t.Fatal("expected invalid compressed data to fail")
					}
				})
			}
		})
	}
	_, closeDecoders, err := decodedBodyReader(strings.NewReader("body"), http.Header{"Content-Encoding": {"br, zstd"}})
	defer closeDecoders()
	if !errors.Is(err, errUnsupportedContentEncoding) {
		t.Fatalf("expected unsupported encoding error, got %v", err)
	}
}

package runtime

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/xml"
	"errors"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
	"io"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// Only protocol-owned resource hrefs are mapped. Lock tokens, owner content and
// arbitrary dead-property payloads are opaque. Original XML bytes, namespace
// prefixes and extension attributes are copied around the changed URI text.
func davResourceHref(stack []xml.Name) bool {
	for _, route := range []string{
		"multistatus/response/href", "multistatus/response/location/href",
		"prop/lockdiscovery/activelock/lockroot/href",
		"multistatus/response/propstat/prop/lockdiscovery/activelock/lockroot/href",
		"error/lock-token-submitted/href", "error/no-conflicting-lock/href",
		"multistatus/response/error/lock-token-submitted/href", "multistatus/response/error/no-conflicting-lock/href",
	} {
		parts := strings.Split(route, "/")
		if len(parts) != len(stack) {
			continue
		}
		matches := true
		for i, part := range parts {
			if stack[i] != (xml.Name{Space: "DAV:", Local: part}) {
				matches = false
				break
			}
		}
		if matches {
			return true
		}
	}
	return false
}

var xmlEncodingDeclaration = regexp.MustCompile(`(?i)encoding\s*=\s*(?:"[^"]*"|'[^']*')`)

const davXMLTokenLimit = 1 << 20

// Bound decoder read-ahead as well as the parsed token: checking only after
// Token returns would already have allocated the entire attacker-sized token.
type davXMLPending struct{ bytes.Buffer }

func (b *davXMLPending) Write(p []byte) (int, error) {
	if b.Len()+len(p) > davXMLTokenLimit+4096 {
		return 0, errors.New("DAV XML token exceeds limit")
	}
	return b.Buffer.Write(p)
}

func mapDAVXML(dst io.Writer, src io.Reader, mapper davMapper, base *url.URL, decodedUTF16 ...bool) error {
	var pending davXMLPending
	decoder := xml.NewDecoder(io.TeeReader(src, &pending))
	converted := len(decodedUTF16) > 0 && decodedUTF16[0]
	if converted {
		decoder.CharsetReader = func(_ string, input io.Reader) (io.Reader, error) { return input, nil }
	}
	var stack []xml.Name
	var previous int64
	var href strings.Builder
	inHref := false
	selfClosedHref := ""
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			_, err = io.Copy(dst, &pending)
			return err
		}
		if err != nil {
			return err
		}
		offset := decoder.InputOffset()
		consumed := int(offset - previous)
		if consumed > davXMLTokenLimit {
			return errors.New("DAV XML token exceeds limit")
		}
		previous = offset
		if consumed > pending.Len() {
			return errors.New("invalid XML offset")
		}
		raw := pending.Next(consumed)
		skip := false
		switch t := token.(type) {
		case xml.ProcInst:
			if converted && t.Target == "xml" {
				raw = xmlEncodingDeclaration.ReplaceAll(raw, []byte(`encoding="UTF-8"`))
			}
		case xml.StartElement:
			if len(stack) >= 256 {
				return errors.New("DAV XML nesting exceeds limit")
			}
			if inHref {
				return errors.New("nested DAV resource href")
			}
			stack = append(stack, t.Name)
			if davResourceHref(stack) {
				inHref = true
				href.Reset()
				if bytes.HasSuffix(raw, []byte("/>")) {
					nameEnd := bytes.IndexAny(raw[1:], " \t\r\n/>") + 1
					selfClosedHref = string(raw[1:nameEnd])
					raw = append(append([]byte(nil), raw[:len(raw)-2]...), '>')
				}
			}
		case xml.CharData:
			if inHref {
				if href.Len()+len(t) > 16<<10 {
					return errors.New("DAV resource href exceeds limit")
				}
				href.Write([]byte(t))
				skip = true
			}
		case xml.EndElement:
			if inHref {
				mapped, e := mapper.fromUpstream(strings.TrimSpace(href.String()), base)
				if e != nil {
					return e
				}
				if e = xml.EscapeText(dst, []byte(mapped)); e != nil {
					return e
				}
				inHref = false
				if selfClosedHref != "" {
					raw = []byte("</" + selfClosedHref + ">")
					selfClosedHref = ""
				}
			}
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		}
		if !skip {
			if _, err = dst.Write(raw); err != nil {
				return err
			}
		}
	}
}

type davXMLBody struct {
	*io.PipeReader
	upstream io.ReadCloser
}

func (b *davXMLBody) Close() error { _ = b.PipeReader.Close(); return b.upstream.Close() }

func rewriteDAVXML(response *http.Response, mapper davMapper, base *url.URL) error {
	media, params, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || (media != "application/xml" && media != "text/xml" && !strings.HasSuffix(media, "+xml")) {
		return nil
	}
	// GET returns stored user content, even if it happens to contain DAV XML.
	if response.Request != nil && (response.Request.Method == "GET" || response.Request.Method == "HEAD") {
		return nil
	}
	original := response.Body
	var source io.Reader = original
	var compressed *gzip.Reader
	switch strings.ToLower(strings.TrimSpace(response.Header.Get("Content-Encoding"))) {
	case "", "identity":
	case "gzip":
		compressed, err = gzip.NewReader(original)
		if err != nil {
			return err
		}
		source = compressed
	default:
		return errors.New("unsupported DAV XML content encoding")
	}
	buffered := bufio.NewReader(source)
	prefix, _ := buffered.Peek(4)
	charset := strings.ToLower(params["charset"])
	converted := strings.HasPrefix(charset, "utf-16") || bytes.HasPrefix(prefix, []byte{0xff, 0xfe}) || bytes.HasPrefix(prefix, []byte{0xfe, 0xff}) || bytes.HasPrefix(prefix, []byte{'<', 0, '?', 0}) || bytes.HasPrefix(prefix, []byte{0, '<', 0, '?'})
	source = buffered
	if converted {
		order := unicode.BigEndian
		if charset == "utf-16le" || bytes.HasPrefix(prefix, []byte{0xff, 0xfe}) || bytes.HasPrefix(prefix, []byte{'<', 0}) {
			order = unicode.LittleEndian
		}
		source = transform.NewReader(buffered, unicode.UTF16(order, unicode.UseBOM).NewDecoder())
		params["charset"] = "utf-8"
		response.Header.Set("Content-Type", mime.FormatMediaType(media, params))
	}
	reader, writer := io.Pipe()
	go func() {
		defer original.Close()
		if compressed != nil {
			defer compressed.Close()
		}
		err := mapDAVXML(writer, source, mapper, base, converted)
		_ = writer.CloseWithError(err)
	}()
	response.Body = &davXMLBody{PipeReader: reader, upstream: original}
	response.ContentLength = -1
	response.Header.Del("Content-Length")
	response.Header.Del("Content-Encoding")
	response.Header.Del("Content-MD5")
	response.Header.Del("Digest")
	response.Header.Del("Content-Digest")
	return nil
}

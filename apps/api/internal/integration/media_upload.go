package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
)

// maxAssetBytes bounds how much of an upload is read into memory before the
// request is built. The HTTP handler enforces the same limit first; this is
// the byte-level backstop so no caller can balloon this method.
const maxAssetBytes = 32 << 20

// AssetUpload describes one uploaded Sanity image asset.
type AssetUpload struct {
	// Ref is the asset document _id used for Portable Text image references,
	// e.g. "image-6b3f...-800x600-jpg".
	Ref string
	// URL is the CDN URL for browser previews.
	URL string
}

// UploadImage streams one image file into the project's dataset via Sanity's
// asset endpoint and returns the asset reference plus its CDN URL. It is a
// write to the connected project; the token stays server-side.
func (s SanityOps) UploadImage(ctx context.Context, filename string, r io.Reader) (AssetUpload, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxAssetBytes+1))
	if err != nil {
		return AssetUpload{}, &SanityWriteError{Kind: SanityFailNetwork}
	}
	if len(data) > maxAssetBytes {
		return AssetUpload{}, &SanityWriteError{
			Kind: SanityFailProvider, HTTP: http.StatusRequestEntityTooLarge,
			Text: "image too large",
		}
	}

	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)
	part, err := writer.CreateFormFile("file", filenamePart(filename))
	if err != nil {
		return AssetUpload{}, &SanityWriteError{Kind: SanityFailMalformed}
	}
	if _, err := part.Write(data); err != nil {
		return AssetUpload{}, &SanityWriteError{Kind: SanityFailNetwork}
	}
	if err := writer.Close(); err != nil {
		return AssetUpload{}, &SanityWriteError{Kind: SanityFailMalformed}
	}

	url := fmt.Sprintf("%s/v1/assets/images/%s", s.apiBase(), s.Creds.Dataset)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(buffer.Bytes()))
	if err != nil {
		return AssetUpload{}, &SanityWriteError{Kind: SanityFailNetwork}
	}
	req.Header.Set("Authorization", "Bearer "+s.Creds.Token)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := s.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
			return AssetUpload{}, &SanityWriteError{Kind: SanityFailTimeout}
		}
		return AssetUpload{}, &SanityWriteError{Kind: SanityFailNetwork}
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return AssetUpload{}, &SanityWriteError{Kind: SanityFailMalformed}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return AssetUpload{}, &SanityWriteError{
			Kind: SanityFailProvider, HTTP: resp.StatusCode, Text: firstSafe(body),
		}
	}

	var result struct {
		Document struct {
			ID string `json:"_id"`
		} `json:"document"`
		URL string `json:"url"`
	}
	if err := json.Unmarshal(body, &result); err != nil || result.Document.ID == "" {
		return AssetUpload{}, &SanityWriteError{Kind: SanityFailMalformed}
	}
	return AssetUpload{Ref: result.Document.ID, URL: result.URL}, nil
}

// filenamePart keeps a safe fallback name so multipart encoding always has a
// filename; Sanity derives the asset id from the uploaded bytes, not the name.
func filenamePart(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "upload.bin"
	}
	return name
}

package internals

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"strings"
	"time"
)

const maxStoredImageBytes = 5 << 20
const maxFetchedImageBytes = 16 << 20

func decodeProductUpload(encoded string) ([]byte, error) {
	encoded = strings.TrimSpace(encoded)
	if encoded == "" {
		return nil, nil
	}
	if i := strings.Index(encoded, ","); i >= 0 && strings.Contains(encoded[:i], "base64") {
		encoded = encoded[i+1:]
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, errors.New("uploaded image could not be read")
	}
	if len(raw) > maxFetchedImageBytes {
		return nil, errors.New("image is too large; choose an image under 16 MB")
	}
	return raw, nil
}

func fetchProductImage(ctx context.Context, imageURL string) ([]byte, string, error) {
	imageURL = strings.TrimSpace(imageURL)
	if imageURL == "" {
		return nil, "", nil
	}
	if !strings.HasPrefix(imageURL, "https://") && !strings.HasPrefix(imageURL, "http://") {
		return nil, "", errors.New("image URL must start with http:// or https://")
	}
	client := &http.Client{Timeout: 18 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return nil, "", errors.New("image URL could not be opened")
	}
	req.Header.Set("User-Agent", "Lixr/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", errors.New("image could not be downloaded from that URL")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("image URL returned status %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxFetchedImageBytes+1))
	if err != nil {
		return nil, "", errors.New("image download was interrupted")
	}
	if len(raw) > maxFetchedImageBytes {
		return nil, "", errors.New("image is too large; choose an image under 16 MB")
	}
	mimeType := strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0])
	return raw, mimeType, nil
}

func optimizeProductImage(raw []byte, hintedMime string) ([]byte, string, error) {
	if len(raw) == 0 {
		return nil, "", nil
	}
	if len(raw) > maxFetchedImageBytes {
		return nil, "", errors.New("image is too large")
	}

	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		mimeType := hintedMime
		if mimeType == "" {
			mimeType = http.DetectContentType(raw)
		}
		if !strings.HasPrefix(mimeType, "image/") {
			return nil, "", errors.New("the selected file is not a supported image")
		}
		if len(raw) > maxStoredImageBytes {
			return nil, "", errors.New("this image format cannot be reduced below 5 MB; choose a smaller JPEG, PNG, GIF, or another image under 5 MB")
		}
		return raw, mimeType, nil
	}

	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	const maxDimension = 2400
	if w > maxDimension || h > maxDimension {
		ratio := float64(maxDimension) / float64(w)
		if h > w {
			ratio = float64(maxDimension) / float64(h)
		}
		nw := int(float64(w) * ratio)
		nh := int(float64(h) * ratio)
		if nw < 1 {
			nw = 1
		}
		if nh < 1 {
			nh = 1
		}
		dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
		for y := 0; y < nh; y++ {
			sy := bounds.Min.Y + y*h/nh
			for x := 0; x < nw; x++ {
				sx := bounds.Min.X + x*w/nw
				dst.Set(x, y, img.At(sx, sy))
			}
		}
		img = dst
	}

	for _, quality := range []int{86, 78, 68, 58, 48} {
		var out bytes.Buffer
		if err := jpeg.Encode(&out, img, &jpeg.Options{Quality: quality}); err != nil {
			return nil, "", errors.New("image could not be optimized")
		}
		if out.Len() <= maxStoredImageBytes {
			return out.Bytes(), "image/jpeg", nil
		}
	}
	return nil, "", errors.New("image remains above 5 MB after optimization; choose a smaller image")
}

func (a *App) handleProductImage(w http.ResponseWriter, r *http.Request) {
	id, err := parsePathID(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	var data []byte
	var mimeType string
	var fallback string
	err = a.db.QueryRowContext(r.Context(), `SELECT image_data,COALESCE(image_mime,''),COALESCE(image_source_url,image_url,'') FROM products WHERE id=?`, id).Scan(&data, &mimeType, &fallback)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if len(data) == 0 {
		if strings.HasPrefix(fallback, "http://") || strings.HasPrefix(fallback, "https://") {
			http.Redirect(w, r, fallback, http.StatusTemporaryRedirect)
			return
		}
		http.NotFound(w, r)
		return
	}
	if mimeType == "" {
		mimeType = http.DetectContentType(data)
	}
	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Header().Set("Content-Length", fmt.Sprint(len(data)))
	_, _ = w.Write(data)
}

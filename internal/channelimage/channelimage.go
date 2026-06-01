// Package channelimage backfills Jellyfin series artwork (poster + backdrop)
// from the channel-level info.json that the Jellyfin-layout bootstrap writes.
//
// The yt-dlp channel dump carries a `thumbnails` array describing the
// channel's avatar (square) and banner (wide) at several resolutions. We
// pick the best of each and write them into the show root as poster.* and
// backdrop.*, which Jellyfin's built-in local image provider consumes
// directly — no plugin or YouTube Data API quota required.
package channelimage

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// Thumbnail mirrors one entry of yt-dlp's `thumbnails` array. Width/Height
// are zero when yt-dlp omits them (common for the banner), in which case we
// fall back to the `id` hint ("avatar"/"banner") to classify the image.
type Thumbnail struct {
	URL    string `json:"url"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	ID     string `json:"id"`
}

// PosterStem and BackdropStem are the on-disk basenames (without extension)
// Jellyfin recognizes for series Primary and Backdrop images respectively.
const (
	PosterStem   = "poster"
	BackdropStem = "backdrop"

	// squareRatioMax is the largest height:width (or width:height) ratio a
	// thumbnail may have and still count as the square channel avatar.
	squareRatioMax = 1.2
	// wideRatioMin is the smallest width:height ratio a thumbnail must have
	// to count as the wide channel banner.
	wideRatioMin = 2.0
)

// ReadThumbnails parses the `thumbnails` array out of a channel info.json.
func ReadThumbnails(path string) ([]Thumbnail, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var doc struct {
		Thumbnails []Thumbnail `json:"thumbnails"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return doc.Thumbnails, nil
}

// SelectPoster picks the channel avatar: prefer a squareish thumbnail whose
// id marks it an avatar, then any squareish thumbnail, then whatever is
// closest to square. Returns false only when no usable thumbnail exists.
func SelectPoster(thumbs []Thumbnail) (Thumbnail, bool) {
	if t, ok := maxArea(thumbs, func(t Thumbnail) bool { return isSquare(t) && hasHint(t.ID, "avatar") }); ok {
		return t, true
	}
	if t, ok := maxArea(thumbs, isSquare); ok {
		return t, true
	}
	return closestToSquare(thumbs)
}

// SelectBackdrop picks the channel banner: prefer a thumbnail whose id marks
// it a banner, then any sufficiently wide thumbnail. Returns false when the
// channel has no banner (common) — callers should treat that as normal and
// skip the backdrop, which Jellyfin handles fine.
func SelectBackdrop(thumbs []Thumbnail) (Thumbnail, bool) {
	if t, ok := maxArea(thumbs, func(t Thumbnail) bool {
		return hasHint(t.ID, "banner") && (isWide(t) || !hasDims(t))
	}); ok {
		return t, true
	}
	return maxArea(thumbs, isWide)
}

func hasDims(t Thumbnail) bool { return t.Width > 0 && t.Height > 0 }

func isSquare(t Thumbnail) bool {
	if !hasDims(t) {
		return false
	}
	hi, lo := t.Width, t.Height
	if lo > hi {
		hi, lo = lo, hi
	}
	return float64(hi)/float64(lo) <= squareRatioMax
}

func isWide(t Thumbnail) bool {
	if !hasDims(t) {
		return false
	}
	return float64(t.Width)/float64(t.Height) >= wideRatioMin
}

func hasHint(id, sub string) bool {
	return strings.Contains(strings.ToLower(id), sub)
}

// maxArea returns the URL-bearing thumbnail with the greatest pixel area
// among those satisfying pred. Thumbnails without dimensions have area 0 but
// are still eligible (used for dimensionless banners).
func maxArea(thumbs []Thumbnail, pred func(Thumbnail) bool) (Thumbnail, bool) {
	var best Thumbnail
	var bestArea int64 = -1
	found := false
	for _, t := range thumbs {
		if t.URL == "" || !pred(t) {
			continue
		}
		area := int64(t.Width) * int64(t.Height)
		if area > bestArea {
			bestArea, best, found = area, t, true
		}
	}
	return best, found
}

// closestToSquare is the last-resort poster fallback: the dimensioned
// thumbnail with the most square aspect ratio, breaking ties by area.
func closestToSquare(thumbs []Thumbnail) (Thumbnail, bool) {
	var best Thumbnail
	bestRatio := math.MaxFloat64
	var bestArea int64 = -1
	found := false
	for _, t := range thumbs {
		if t.URL == "" || !hasDims(t) {
			continue
		}
		hi, lo := t.Width, t.Height
		if lo > hi {
			hi, lo = lo, hi
		}
		ratio := float64(hi) / float64(lo)
		area := int64(t.Width) * int64(t.Height)
		if ratio < bestRatio || (ratio == bestRatio && area > bestArea) {
			bestRatio, bestArea, best, found = ratio, area, t, true
		}
	}
	return best, found
}

// Existing returns the path of an already-written image with the given stem
// (poster/backdrop) in destDir, trying the known image extensions, or ""
// when none exists. Used for idempotency.
func Existing(destDir, stem string) string {
	for _, ext := range []string{".jpg", ".jpeg", ".png", ".webp"} {
		p := filepath.Join(destDir, stem+ext)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// Download fetches url and writes it into destDir as stem + the extension
// implied by the response content type / magic bytes. It is a no-op (written
// == false) when an image with that stem already exists and force is false.
func Download(ctx context.Context, hc *http.Client, url, destDir, stem string, force bool) (path string, written bool, err error) {
	if existing := Existing(destDir, stem); existing != "" && !force {
		return existing, false, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", false, fmt.Errorf("build request: %w", err)
	}
	resp, err := hc.Do(req)
	if err != nil {
		return "", false, fmt.Errorf("get %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", false, fmt.Errorf("get %s: status %d", url, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", false, fmt.Errorf("read body %s: %w", url, err)
	}

	ext := imageExt(resp.Header.Get("Content-Type"), body)
	dst := filepath.Join(destDir, stem+ext)

	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", false, fmt.Errorf("mkdir %s: %w", destDir, err)
	}
	// When forcing, clear any prior image with this stem (possibly a
	// different extension) so we don't leave two posters behind.
	if force {
		if old := Existing(destDir, stem); old != "" && old != dst {
			_ = os.Remove(old)
		}
	}
	if err := os.WriteFile(dst, body, 0o644); err != nil {
		return "", false, fmt.Errorf("write %s: %w", dst, err)
	}
	return dst, true, nil
}

// imageExt maps a content type (falling back to magic-byte sniffing) to a
// Jellyfin-friendly extension, defaulting to .jpg since most YouTube channel
// art is JPEG.
func imageExt(contentType string, body []byte) string {
	switch ct := strings.ToLower(contentType); {
	case strings.Contains(ct, "png"):
		return ".png"
	case strings.Contains(ct, "webp"):
		return ".webp"
	case strings.Contains(ct, "jpeg"), strings.Contains(ct, "jpg"):
		return ".jpg"
	}
	switch {
	case len(body) >= 2 && body[0] == 0xFF && body[1] == 0xD8:
		return ".jpg"
	case len(body) >= 8 && string(body[:8]) == "\x89PNG\r\n\x1a\n":
		return ".png"
	case len(body) >= 12 && string(body[:4]) == "RIFF" && string(body[8:12]) == "WEBP":
		return ".webp"
	}
	return ".jpg"
}

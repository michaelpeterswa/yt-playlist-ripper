package channelimage

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// realisticThumbs mirrors the shape of a yt-dlp channel dump: several square
// avatar variants (some id-tagged), one wide banner, and a couple of
// dimensionless entries.
func realisticThumbs() []Thumbnail {
	return []Thumbnail{
		{URL: "https://x/avatar48", Width: 48, Height: 48, ID: "0"},
		{URL: "https://x/avatar160", Width: 160, Height: 160, ID: "1"},
		{URL: "https://x/avatar900", Width: 900, Height: 900, ID: "avatar_uncropped"},
		{URL: "https://x/banner", Width: 2560, Height: 424, ID: "banner_uncropped"},
	}
}

func TestSelectPosterPrefersSquareAvatar(t *testing.T) {
	got, ok := SelectPoster(realisticThumbs())
	if !ok {
		t.Fatal("expected a poster")
	}
	if got.URL != "https://x/avatar900" {
		t.Errorf("poster = %q; want the 900x900 avatar", got.URL)
	}
}

func TestSelectPosterFallsBackToLargestSquare(t *testing.T) {
	// No avatar-tagged thumbnails: still want the biggest square one.
	thumbs := []Thumbnail{
		{URL: "https://x/s100", Width: 100, Height: 100, ID: "a"},
		{URL: "https://x/s512", Width: 512, Height: 512, ID: "b"},
		{URL: "https://x/wide", Width: 1280, Height: 200, ID: "banner_uncropped"},
	}
	got, ok := SelectPoster(thumbs)
	if !ok {
		t.Fatal("expected a poster")
	}
	if got.URL != "https://x/s512" {
		t.Errorf("poster = %q; want the 512 square", got.URL)
	}
}

func TestSelectPosterClosestToSquareWhenNoneSquare(t *testing.T) {
	thumbs := []Thumbnail{
		{URL: "https://x/2to1", Width: 200, Height: 100, ID: "a"},   // ratio 2.0
		{URL: "https://x/4to3", Width: 400, Height: 300, ID: "b"},   // ratio 1.33
		{URL: "https://x/16to9", Width: 1600, Height: 900, ID: "c"}, // ratio 1.78
	}
	got, ok := SelectPoster(thumbs)
	if !ok {
		t.Fatal("expected a poster")
	}
	if got.URL != "https://x/4to3" {
		t.Errorf("poster = %q; want the 4:3 (closest to square)", got.URL)
	}
}

func TestSelectBackdropPrefersBanner(t *testing.T) {
	got, ok := SelectBackdrop(realisticThumbs())
	if !ok {
		t.Fatal("expected a backdrop")
	}
	if got.URL != "https://x/banner" {
		t.Errorf("backdrop = %q; want the banner", got.URL)
	}
}

func TestSelectBackdropDimensionlessBanner(t *testing.T) {
	// Banner with no width/height should still be chosen via its id hint.
	thumbs := []Thumbnail{
		{URL: "https://x/avatar", Width: 900, Height: 900, ID: "avatar_uncropped"},
		{URL: "https://x/banner", ID: "banner_uncropped"},
	}
	got, ok := SelectBackdrop(thumbs)
	if !ok {
		t.Fatal("expected a backdrop")
	}
	if got.URL != "https://x/banner" {
		t.Errorf("backdrop = %q; want the dimensionless banner", got.URL)
	}
}

func TestSelectBackdropNoneWhenOnlySquares(t *testing.T) {
	thumbs := []Thumbnail{
		{URL: "https://x/a", Width: 100, Height: 100, ID: "0"},
		{URL: "https://x/b", Width: 900, Height: 900, ID: "avatar_uncropped"},
	}
	if _, ok := SelectBackdrop(thumbs); ok {
		t.Error("expected no backdrop when the channel has no banner")
	}
}

func TestReadThumbnails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Chan - NA - Videos [UCabc].info.json")
	body := `{"id":"NA","uploader":"Chan","thumbnails":[
		{"url":"https://x/a","width":900,"height":900,"id":"avatar_uncropped"},
		{"url":"https://x/b","width":2560,"height":424,"id":"banner_uncropped"}
	]}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	thumbs, err := ReadThumbnails(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(thumbs) != 2 {
		t.Fatalf("got %d thumbnails, want 2", len(thumbs))
	}
}

func TestDownloadWritesAndIsIdempotent(t *testing.T) {
	jpeg := []byte{0xFF, 0xD8, 0xFF, 0xE0, 'd', 'a', 't', 'a'}
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write(jpeg)
	}))
	defer srv.Close()

	dir := t.TempDir()
	hc := &http.Client{Timeout: 5 * time.Second}
	ctx := context.Background()

	path, written, err := Download(ctx, hc, srv.URL, dir, PosterStem, false)
	if err != nil {
		t.Fatal(err)
	}
	if !written {
		t.Fatal("expected first download to write")
	}
	if filepath.Base(path) != "poster.jpg" {
		t.Errorf("path = %q; want poster.jpg", path)
	}

	// Second call without force must be a no-op (no extra HTTP hit).
	_, written2, err := Download(ctx, hc, srv.URL, dir, PosterStem, false)
	if err != nil {
		t.Fatal(err)
	}
	if written2 {
		t.Error("expected second download to skip (idempotent)")
	}
	if hits != 1 {
		t.Errorf("server hit %d times; want 1 (idempotent skip should not re-fetch)", hits)
	}
}

func TestDownloadForceOverwrites(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("\x89PNG\r\n\x1a\nrest"))
	}))
	defer srv.Close()

	dir := t.TempDir()
	// Pre-existing jpg poster from a prior run.
	if err := os.WriteFile(filepath.Join(dir, "poster.jpg"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	hc := &http.Client{Timeout: 5 * time.Second}
	path, written, err := Download(context.Background(), hc, srv.URL, dir, PosterStem, true)
	if err != nil {
		t.Fatal(err)
	}
	if !written {
		t.Fatal("expected force download to write")
	}
	if filepath.Base(path) != "poster.png" {
		t.Errorf("path = %q; want poster.png", path)
	}
	// The stale poster.jpg should have been removed to avoid two posters.
	if _, err := os.Stat(filepath.Join(dir, "poster.jpg")); err == nil {
		t.Error("expected stale poster.jpg to be removed on force")
	}
}

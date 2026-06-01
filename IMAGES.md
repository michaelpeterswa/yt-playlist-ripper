# Channel Artwork for the Jellyfin Layout

How yt-playlist-ripper gives each channel a **poster** (avatar) and
**backdrop** (banner) in Jellyfin, and how to backfill artwork for channels
that were synced before this feature existed.

> **History:** an earlier draft of this document proposed renaming every
> folder to `Channel [UCxxxx]` and writing `tvshow.nfo` files via a Python
> script. That approach was **not** what shipped. The Jellyfin layout instead
> keys off a channel-level `info.json` consumed by the
> [jellyfin-youtube-metadata-plugin](https://github.com/ankenyr/jellyfin-youtube-metadata-plugin),
> and folders stay as plain `{uploader}/` (the channel ID lives in the
> info.json filename, not the folder name). This document describes that
> shipped design.

---

## How it works

When `JELLYFIN_LAYOUT=true`, every channel that gets synced is first
*bootstrapped*: a metadata-only yt-dlp pass writes a channel-level
`info.json` at the show root:

```
{OUTPUT_ROOT}/{uploader}/
├── {uploader} - NA - {playlist_title} [{channel_id}].info.json   ← channel metadata
└── Season {YYYY}/
    └── {uploader} - {YYYYMMDD} - {title} [{id}].{mkv,info.json,jpg,...}
```

That channel `info.json` carries a `thumbnails` array describing the
channel's **avatar** (square) and **banner** (wide) at several resolutions.
From it we derive two image files written next to the info.json:

- **`poster.*`** — highest-resolution square avatar.
- **`backdrop.*`** — widest banner (skipped when the channel has none).

These are plain image files that **Jellyfin's built-in local image provider**
picks up directly — no metadata plugin and no YouTube Data API key required.
The metadata plugin, if installed, still layers richer series metadata on top.

### Selection rules

Implemented in `internal/channelimage`:

- **Poster:** prefer a squareish thumbnail (aspect ratio ≤ 1.2) whose `id`
  marks it an avatar; else the largest squareish thumbnail; else whatever is
  closest to square. The avatar is the channel logo.
- **Backdrop:** prefer a thumbnail whose `id` marks it a banner; else any
  sufficiently wide thumbnail (aspect ratio ≥ 2.0). No banner → no backdrop,
  which Jellyfin handles fine.

The file extension (`.jpg` / `.png` / `.webp`) is chosen from the response
content type / magic bytes; most YouTube channel art is JPEG.

---

## Going forward: automatic

New channels need no action. After a successful bootstrap, the runtime
locates the just-written channel `info.json`, selects the avatar/banner, and
writes `poster.*` / `backdrop.*` into the show root. This is best-effort:
a download failure is logged but never fails the sync, and existing images
are never overwritten. The whole bootstrap (info.json + images) is gated by
the `.bootstrap/{channel_id}.done` sentinel, so it runs once per channel.

To re-fetch a single channel's artwork, delete its sentinel
(`{OUTPUT_ROOT}/.bootstrap/{channel_id}.done`) and let the next cron tick
re-bootstrap it.

---

## Backfilling existing channels: `cmd/backfill-images`

Channels synced before this feature have a channel `info.json` but no
`poster.*` / `backdrop.*`. The `backfill-images` tool reads the `info.json`
already on disk (no YouTube metadata calls — it only downloads the chosen
images) and writes the artwork in place.

Dry-run first to see what it would do:

```bash
go run ./cmd/backfill-images --root /downloads
```

Then apply:

```bash
go run ./cmd/backfill-images --root /downloads --apply
```

| Flag      | Default       | Meaning                                                        |
|-----------|---------------|----------------------------------------------------------------|
| `--root`  | `/downloads`  | Library root to scan (the runtime's `OUTPUT_ROOT`).            |
| `--apply` | `false`       | Actually download images; otherwise dry-run.                  |
| `--force` | `false`       | Overwrite existing `poster.*` / `backdrop.*` (replaces stale art). |

The tool is **idempotent**: by default it skips any channel that already has
the corresponding image, so re-running after a partial run is safe. It scans
for channel-level info.json files (those containing the ` - NA - ` infix) and
skips the `.bootstrap` / `.archives` state directories.

---

## Verifying in Jellyfin

1. Run the backfill (or let the bootstrap run for new channels).
2. In Jellyfin, the show root now contains `poster.*` and (when available)
   `backdrop.*`. Trigger **Refresh Metadata** on the library so the local
   image provider picks them up.
3. Confirm each channel shows its avatar as the poster and banner as the
   backdrop. Channels with no banner show only a poster — expected.

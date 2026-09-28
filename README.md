<h1 align="center">
	yt-playlist-ripper
</h1>
<h3 align="center">
	yt-playlist-ripper leverages yt-dlp to clone public playlists for archival purposes
</h3>
<p align="center">
	<strong>
		<a href="https://github.com/yt-dlp/yt-dlp">yt-dlp GitHub</a>
		•
		<a href="https://youtube.com">YouTube</a>
	</strong>
</p>
<p align="center">
  <img alt="Made with Go" src=".github/images/made-with-go.svg">
  <img alt="Questionably Legal" src=".github/images/questionably-legal.svg">
</p>

## Deployment

### Docker

```
docker run \
  -d \
  --name='yt-playlist-ripper' \
  -e 'CRON_STRING'='0 */6 * * *' \
  -e 'PLAYLIST_LIST'='PLUcjmvZLvmS8PaBz77N1eFAbJ0cLENSwU' \
  -e 'METRICS_PORT'=8081 \
  -p '8081:8081/tcp' \
  -v '/<folder on your machine>':'/downloads' 'ghcr.io/michaelpeterswa/yt-playlist-ripper:v2.0.5'
```

### Jellyfin-compatible layout

Set `JELLYFIN_LAYOUT=true` to make the playlist runner emit a library shaped
for the
[jellyfin-youtube-metadata-plugin](https://github.com/ankenyr/jellyfin-youtube-metadata-plugin)'s
local provider. The on-disk shape becomes:

```
{OUTPUT_ROOT}/
└── {uploader}/                                       # one Show per channel
    ├── {uploader} - NA - {playlist_title} [{channel_id}].info.json
    ├── {uploader} - NA - {playlist_title} [{channel_id}].jpg   # channel avatar (Show poster)
    └── Season {YYYY}/
        ├── {uploader} - {YYYYMMDD} - {title} [{id}].mkv
        ├── {uploader} - {YYYYMMDD} - {title} [{id}].info.json
        └── {uploader} - {YYYYMMDD} - {title} [{id}].jpg
```

A single playlist can contain videos from many channels — they get split
into the correct show folder by uploader automatically. For every channel
that appears in the playlist, a one-shot metadata pass writes the
channel-level `info.json` plus its avatar `.jpg` at the show root. That
bootstrap is gated by a sentinel under `{OUTPUT_ROOT}/.bootstrap/` so we
don't re-hit YouTube on every tick; delete a sentinel (or the whole
`.bootstrap/` directory) to force a re-bootstrap — e.g. to backfill
missing channel images on a library bootstrapped by an older version.

```
docker run \
  -d \
  --name='yt-playlist-ripper' \
  -e 'CRON_STRING'='0 */6 * * *' \
  -e 'PLAYLIST_LIST'='PLUcjmvZLvmS8PaBz77N1eFAbJ0cLENSwU' \
  -e 'JELLYFIN_LAYOUT'='true' \
  -e 'OUTPUT_ROOT'='/downloads' \
  -p '8081:8081/tcp' \
  -v '/<folder on your machine>':'/downloads' 'ghcr.io/michaelpeterswa/yt-playlist-ripper:latest'
```

`CHANNEL_LIST` works the same way for mirroring whole channels without a
playlist intermediary; it uses the same templates and bootstrap path.

#### Migrating an existing library

If you already have a library in the legacy layout
(`{playlist} - ({uploader})/{YYYYMMDD} - {title}/...`), the
`migrate-to-jellyfin` binary relocates everything into the new shape
without re-downloading. It reads each per-video `.info.json`, moves the
video + info.json + thumbnail + subtitles together, and tears down the
empty legacy directories.

```
# dry-run (default) — prints every planned move, changes nothing
go run ./cmd/migrate-to-jellyfin --root /downloads

# commit
go run ./cmd/migrate-to-jellyfin --root /downloads --apply
```

Flags:

| Flag                          | Default       | Notes                                                                            |
| ----------------------------- | ------------- | -------------------------------------------------------------------------------- |
| `--root`                      | `/downloads`  | Library root to scan                                                             |
| `--apply`                     | `false`       | Without this, the tool is dry-run only                                           |
| `--restrict-filenames`        | `true`        | Match the runtime's `--restrict-filenames` (Jellyfin layout default)             |
| `--keep-empty-dirs`           | `false`       | Skip the cleanup of now-empty legacy per-video / playlist dirs                   |

The migrator is idempotent: re-running it does nothing once everything is
in the new layout. Channel-level `info.json` files (named `... - NA - ...`)
and the runtime's own state directories (`.bootstrap/`, `.archives/`) are
left alone. After migration, set `JELLYFIN_LAYOUT=true` on the ripper and
the next cron tick will bootstrap each channel's show-root `info.json`
automatically.

##### Duplicates and consistency

A few cases worth understanding before you flip `--apply`:

- **Same video under two legacy playlist dirs.** If the same video appears
  in `PlaylistA - (Channel)/.../[abc].mkv` and `PlaylistB - (Channel)/.../[abc].mkv`,
  both legacy paths resolve to the same Jellyfin target. The planner
  detects this at planning time, keeps the lexicographically-first source,
  and reports the rest as a duplicate group (`duplicate_groups=N` in the
  summary). The leftover copies stay at their legacy locations untouched
  for you to inspect and delete manually.
- **Partial prior migration.** If even one sibling file is already present
  at the target path (e.g. a `.mkv` got moved in a previous run but the
  `.jpg` didn't), the migrator skips the *entire* plan rather than
  half-moving it. This preserves the plugin's "byte-identical base name
  across `.mkv` / `.info.json` / `.jpg`" invariant. These appear as
  `conflicts=N` in the summary with a `destination already exists` warning
  listing the blockers.
- **Active downloads during migration.** Stop the ripper before running
  `--apply` — yt-dlp's in-flight `.part` and `.ytdl` files match the
  sibling glob and would get moved out from under it.
- **Mixed-channel collisions.** If two real channels' uploader strings
  sanitize to the same directory name (extremely rare, but possible with
  heavy Unicode), they'd merge into one Show. Inspect the dry-run output
  before applying; the show-root `info.json` files carry distinct
  `[channel_id]` brackets so the plugin will still tell them apart, but
  you'd see two `*.info.json` at the same show root.

#### Runtime duplicate concerns

The runtime path has its own deduplication story to be aware of:

- **A video in multiple playlists** is handled cleanly. Both playlist
  syncs in `JELLYFIN_LAYOUT=true` mode share the same `ARCHIVE_FILE`, so
  the second sync sees the ID in the archive and skips.
- **A channel covered by both `CHANNEL_LIST` and a playlist with
  `JELLYFIN_LAYOUT=true`** is the one to watch. `CHANNEL_LIST` defaults
  to `ARCHIVE_MODE=per-channel` (one archive per channel under
  `{OUTPUT_ROOT}/.archives/`), but `JELLYFIN_LAYOUT` playlists always
  use the global `ARCHIVE_FILE`. The two archives don't see each other,
  so each runner will re-evaluate the overlapping videos every tick. The
  files on disk are protected by `--no-overwrites`, but you'll waste
  bandwidth on format checks. **Set `ARCHIVE_MODE=global` if you mix
  the two modes for the same channel.**
- **Concurrent cron ticks.** Two playlists triggering at the same instant
  hold different lockmap entries, so they run in parallel. If they
  contain the same video, both yt-dlp processes will race; `--no-overwrites`
  prevents on-disk corruption but the duplicated work is real. Stagger
  cron schedules if this matters.
- **Channel renames.** `%(uploader)s` is whatever yt-dlp sees at fetch
  time. If a channel rebrands, new downloads land in a new directory
  while the bootstrap sentinel masks a re-bootstrap of the channel-level
  `info.json`. The split is harmless to the plugin (channel_id in the
  show-root `.info.json` is the real key) but cosmetically ugly. Delete
  `{OUTPUT_ROOT}/.bootstrap/{channelID}.done` to force a re-bootstrap,
  then `mv` the old dir contents.

Tunable env vars:

| Var                       | Default                                                                                                | Notes                                                  |
| ------------------------- | ------------------------------------------------------------------------------------------------------ | ------------------------------------------------------ |
| `JELLYFIN_LAYOUT`         | `false`                                                                                                | Opt-in switch on the playlist runner                   |
| `OUTPUT_ROOT`             | `/downloads`                                                                                           | Root the templates are joined onto                     |
| `JELLYFIN_VIDEO_TEMPLATE` | `%(uploader)s/Season %(upload_date>%Y)s/%(uploader)s - %(upload_date>%Y%m%d)s - %(title)s [%(id)s].%(ext)s` | Per-video output template                              |
| `JELLYFIN_INFO_TEMPLATE`  | `%(uploader)s/%(uploader)s - NA - %(playlist_title)s [%(channel_id)s].%(ext)s`                         | Show-root info.json template                           |
| `ARCHIVE_MODE`            | `per-channel`                                                                                          | `global` shares `ARCHIVE_FILE` across all channels     |
| `SUB_LANGS`               | `en.*`                                                                                                 | Passed to `--sub-langs`                                |
| `WRITE_AUTO_SUBS`         | `true`                                                                                                 | Set to `false` to skip auto-generated captions         |
| `SUB_FORMAT`              | `srt/best`                                                                                             | Passed to `--sub-format`                               |

## Meta

Michael Peters - michael@michaelpeterswa.com
       
## License   
MIT

<!--

Reference Variables

-->

<!-- Badges -->
[questionably-legal-badge]: .github/images/questionably-legal.svg
[made-with-go-badge]: .github/images/made-with-go.svg

<!-- Links -->
[blank-reference-link]: #
[for-the-badge-link]: https://forthebadge.com

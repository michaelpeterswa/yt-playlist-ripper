package ytdl

import (
	"fmt"
	"strings"
)

type Command struct {
	bin  string
	args []string
}

type CommandOption func(*Command)

func NewCommand(bin string, opts ...CommandOption) *Command {
	var command Command

	// prepend binary name
	command.bin = bin

	for _, opt := range opts {
		opt(&command)
	}

	return &command
}

func (c *Command) Bin() string {
	return c.bin
}

func (c *Command) Args() []string {
	return c.args
}

func (c *Command) String() string {
	return fmt.Sprintf("%s %s", c.bin, strings.Join(c.args, " "))
}

// Do not print progress bar
func WithNoProgress() CommandOption {
	return func(cmd *Command) {
		cmd.args = append(cmd.args, "--no-progress")
	}
}

// Write thumbnail image to disk
func WithWriteThumbnail() CommandOption {
	return func(cmd *Command) {
		cmd.args = append(cmd.args, "--write-thumbnail")
	}
}

// Output filename template; see "OUTPUT TEMPLATE" for details
func WithOutputTemplate(template string) CommandOption {
	return func(cmd *Command) {
		cmd.args = append(cmd.args, "--output", template)
	}
}

// Download only videos not listed in the archive file. Record the IDs of all downloaded videos in it
func WithDownloadArchive(archive string) CommandOption {
	return func(cmd *Command) {
		cmd.args = append(cmd.args, "--download-archive", archive)
	}
}

// Video format code, see "FORMAT SELECTION" for more details
func WithFormat(format string) CommandOption {
	return func(cmd *Command) {
		cmd.args = append(cmd.args, "--format", format)
	}
}

// Print various debugging information
func WithVerbose() CommandOption {
	return func(cmd *Command) {
		cmd.args = append(cmd.args, "--verbose")
	}
}

// Activate quiet mode. If used with --verbose, print the log to stderr
func WithQuiet() CommandOption {
	return func(cmd *Command) {
		cmd.args = append(cmd.args, "--quiet")
	}
}

// Make all connections via IPv4
func WithForceIPv4() CommandOption {
	return func(cmd *Command) {
		cmd.args = append(cmd.args, "--force-ipv4")
	}
}

// Number of seconds to sleep between requests during data extraction
func WithSleepRequests(seconds int) CommandOption {
	return func(cmd *Command) {
		cmd.args = append(cmd.args, "--sleep-requests", fmt.Sprintf("%d", seconds))
	}
}

// Number of seconds to sleep before each download. This is the minimum time to sleep when used along with --max-sleep-interval (Alias: --min-sleep-interval)
func WithSleepInterval(seconds int) CommandOption {
	return func(cmd *Command) {
		cmd.args = append(cmd.args, "--sleep-interval", fmt.Sprintf("%d", seconds))
	}
}

// Maximum number of seconds to sleep. Can only be used along with --min-sleep-interval
func WithMaxSleepInterval(seconds int) CommandOption {
	return func(cmd *Command) {
		cmd.args = append(cmd.args, "--max-sleep-interval", fmt.Sprintf("%d", seconds))
	}
}

// Ignore download and postprocessing errors. The download will be considered successful even if the postprocessing fails
func WithIgnoreErrors() CommandOption {
	return func(cmd *Command) {
		cmd.args = append(cmd.args, "--ignore-errors")
	}
}

// Do not resume partially downloaded fragments. If the file is not fragmented, restart download of the entire file
func WithNoContinue() CommandOption {
	return func(cmd *Command) {
		cmd.args = append(cmd.args, "--no-continue")
	}
}

// Do not overwrite any files
func WithNoOverwrites() CommandOption {
	return func(cmd *Command) {
		cmd.args = append(cmd.args, "--no-overwrites")
	}
}

// Embed metadata to the video file. Also embeds chapters/infojson if present unless --no-embed-chapters/--no-embed-info-json are used (Alias: --add-metadata)
func WithAddMetadata() CommandOption {
	return func(cmd *Command) {
		cmd.args = append(cmd.args, "--add-metadata")
	}
}

// Parse additional metadata like title/artist from other fields; see "MODIFYING METADATA" for details. Supported values of "WHEN" are the same as that of --use-postprocessor (default: pre_process)
func WithParseMetadata(metadata string) CommandOption {
	return func(cmd *Command) {
		cmd.args = append(cmd.args, "--parse-metadata", metadata)
	}
}

// Write video description to a .description file
func WithWriteDescription() CommandOption {
	return func(cmd *Command) {
		cmd.args = append(cmd.args, "--write-description")
	}
}

// Write video metadata to a .info.json file (this may contain personal information)
func WithWriteInfoJSON() CommandOption {
	return func(cmd *Command) {
		cmd.args = append(cmd.args, "--write-info-json")
	}
}

// Embed thumbnail in the video as cover art
func WithEmbedThumbnail() CommandOption {
	return func(cmd *Command) {
		cmd.args = append(cmd.args, "--embed-thumbnail")
	}
}

// --sub-langs all --write-subs
//
// No longer recommended (https://github.com/yt-dlp/yt-dlp#not-recommended)
func WithAllSubs() CommandOption {
	return func(cmd *Command) {
		cmd.args = append(cmd.args, "--all-subs")
	}
}

// Embed subtitles in the video (only for mp4, webm, and mkv videos)
func WithEmbedSubs() CommandOption {
	return func(cmd *Command) {
		cmd.args = append(cmd.args, "--embed-subs")
	}
}

// Make sure formats are selected only from those that are actually downloadable
func WithCheckFormats() CommandOption {
	return func(cmd *Command) {
		cmd.args = append(cmd.args, "--check-formats")
	}
}

// Number of fragments of a dash/hlsnative video that should be downloaded concurrently (default is 1)
func WithConcurrentFragments(count int) CommandOption {
	return func(cmd *Command) {
		cmd.args = append(cmd.args, "--concurrent-fragments", fmt.Sprintf("%d", count))
	}
}

// Generic video filter.
//
// See https://github.com/yt-dlp/yt-dlp#video-selection for more details
func WithMatchFilter(filter string) CommandOption {
	return func(cmd *Command) {
		cmd.args = append(cmd.args, "--match-filter", filter)
	}
}

// Containers that may be used when merging formats, separated by "/", e.g. "mp4/mkv". Ignored if no merge is required. (currently supported: avi, flv, mkv, mov, mp4, webm)
func WithMergeOutputFormat(format string) CommandOption {
	return func(cmd *Command) {
		cmd.args = append(cmd.args, "--merge-output-format", format)
	}
}

// Download only videos uploaded on or before this date. The date formats accepted are the same as --date
func WithDateBefore(date string) CommandOption {
	return func(cmd *Command) {
		cmd.args = append(cmd.args, "--datebefore", date)
	}
}

// Minimum download rate in bytes per second below which throttling is assumed and the video data is re-extracted, e.g. 100K
func WithThrottledRate(rate string) CommandOption {
	return func(cmd *Command) {
		cmd.args = append(cmd.args, "--throttled-rate", rate)
	}
}

func WithString(s string) CommandOption {
	return func(cmd *Command) {
		cmd.args = append(cmd.args, s)
	}
}

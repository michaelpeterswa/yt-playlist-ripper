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
  -e 'YTPR_CRON_STRING'='0 */6 * * *' \
  -e 'YTPR_PLAYLIST_LIST'='PLUcjmvZLvmS8PaBz77N1eFAbJ0cLENSwU' \
  -e 'YTPR_HTTP_PORT'=8081 \
  -p '9201:8080/tcp' \
  -v '/<folder on your machine>':'/downloads':'rw,slave' 'ghcr.io/michaelpeterswa/yt-playlist-ripper:v2.0.2'
```

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
# Torrent WebDAV Client

> [!WARNING]  
> Although the program is tested and works, it was not designed with a focus on actual use. This is a proof-of-concept torrent client(based on the [anacrolix/torrent](https://github.com/anacrolix/torrent) library) with the ability to stream content over the network. The idea was successful, but I realized that it would be better to mount torrent via FUSE. So I do not intend to further develop this project. This is still a good example of using the library and implementing a WebDav server.

https://github.com/Jipok/torrent-webdav/assets/25588359/f1086431-b5bf-4f9d-8983-6ca593b5252f

## Features

### Automatic Torrent Download

One of the standout features of Torrent WebDAV Client is its ability to automatically start downloading any torrent file that is added to the system. This means that users can simply add a torrent file to the designated directory, and the application will take care of the rest, downloading the file in the background.

### Streaming Partially Downloaded Files

Another key feature is the ability to stream files that are not yet fully downloaded. This is particularly useful for accessing large files that are still being downloaded, allowing users to start using the file before it is completely downloaded. This feature is powered by the integration of a WebDAV server, which enables seamless file access over the network.

### WebDAV Server Integration

The application includes a built-in WebDAV server, which allows users to access their torrent files over a network. This means that users can access their files from any device that supports WebDAV, making it easy to share and collaborate on files.

### Easy Management of Torrents

Torrent WebDAV Client provides a simple and intuitive interface for managing torrents. Users can easily add new torrent files, pause and resume downloads, and remove torrents from the system. Everything through the usual actions with the file system.

## Getting Started

To get started with Torrent WebDAV Client, simply download the application and run it on your system. The application will automatically start watching the specified directory for new torrent files and begin downloading them. Users can then access their files through the built-in WebDAV server with web ui.

```
Usage of ./trnt2webdav:
  -l string
    	interface:port for WebDav server to listen (default "127.0.0.1:8080")
  -metadata string
    	path to the folder for storing torrents metadata (default "metadata")
  -pass string
    	HTTP Basic Auth Password
  -s string
    	secret URL path for WebDav access
  -torrents string
    	path to folder for store/watch *.torrent files and magnets.txt (default "torrents")
  -user string
    	HTTP Basic Auth Username. if empty, no auth
  -v	Verbose - print DBG messages
```

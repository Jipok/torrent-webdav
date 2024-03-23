package main

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/fsnotify/fsnotify"
	"github.com/rs/zerolog/log"
)

var Watcher *fsnotify.Watcher

func initWatcher() {
	var err error
	Watcher, err = fsnotify.NewWatcher()
	if err != nil {
		log.Fatal().Err(err).Msg("fsnotify.NewWatcher:")
	}

	// Start listening for events
	go func() {
		for {
			select {
			case event, ok := <-Watcher.Events:
				if !ok {
					return
				}
				if event.Has(fsnotify.Write) {
					if strings.HasSuffix(event.Name, "/magnet.txt") {
						parseMagnetsFile(event.Name)
					}
				}

				if strings.HasSuffix(event.Name, "/this.torrent") {
					if event.Has(fsnotify.Remove) {
						path := filepath.Dir(event.Name)
						dropTorrent(path)
						continue
					}
				}

				// Skip all other events in torrent dir
				_, err := os.Stat(filepath.Dir(event.Name) + "/this.torrent")
				if err == nil {
					continue
				}

				if event.Has(fsnotify.Create) {
					stat, err := os.Stat(event.Name)
					if err != nil {
						log.Error().Err(err).Str("Name", event.Name).Msg("Watcher Error")
						continue
					}
					if stat.IsDir() {
						recursiveScanDir(event.Name)
					} else {
						if strings.HasSuffix(event.Name, ".torrent") {
							handleNewTorrentFile(event.Name)
						}
					}
				}

				if event.Has(fsnotify.Remove) || event.Has(fsnotify.Rename) {
					_, err := os.Stat(event.Name)
					if err != nil {
						dropTorrent(event.Name)
					}
				}

			case err, ok := <-Watcher.Errors:
				if !ok {
					return
				}
				log.Error().Err(err).Msg("Error in watcher:")
			}
		}
	}()
}

func recursiveScanDir(path string) bool {
	err := Watcher.Add(path)
	if err != nil {
		log.Error().Str("Path", path).Err(err).Msg("Cant add to watcher:")
	}

	_, err = os.Stat(path + "/this.torrent")
	if err == nil {
		log.Info().Str("Path", path).Msg("Found torrent")
		AddTorrentFile(path + "/this.torrent")
		return false
	}

	log.Info().Str("Path", path).Msg("Watching dir")

	_, err = os.Stat(path + "/magnet.txt")
	if err == nil {
		parseMagnetsFile(path + "/magnet.txt")
	}

	files, err := os.ReadDir(path)
	if err != nil {
		log.Fatal().Str("Path", path).Err(err).Msg("Cant read dir")
		return false
	}
	// Searching for new torrents since the server shutdown
	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".torrent") {
			handleNewTorrentFile(path + "/" + file.Name())
		}
	}
	for _, file := range files {
		if file.IsDir() {
			recursiveScanDir(path + "/" + file.Name())
		}
	}
	return true
}

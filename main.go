package main

import (
	"bufio"
	"flag"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var (
	WebDavAddr string
	WebDavPath string
	Username   string
	Password   string

	MetaDataDir string
	TorrentsDir string

	TorrentClient *torrent.Client
	Server        *WebDAVServer
	Verbose       bool
)

func ensureDirExists(path string) bool {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		err := os.MkdirAll(path, os.ModePerm)
		if err != nil {
			log.Fatal().Str("Path", path).Err(err).Msg("Can't create dir")
		}
		return true
	}
	return false
}

func main() {
	flag.StringVar(&WebDavAddr, "l", "127.0.0.1:8080", "interface:port for WebDav server to listen")
	flag.StringVar(&WebDavPath, "s", "", "secret URL path for WebDav access")
	flag.StringVar(&Username, "user", "", "HTTP Basic Auth Username. if empty, no auth")
	flag.StringVar(&Password, "pass", "", "HTTP Basic Auth Password")
	flag.StringVar(&MetaDataDir, "metadata", "metadata", "path to the folder for storing torrents metadata")
	flag.StringVar(&TorrentsDir, "torrents", "torrents", "path to folder for store/watch *.torrent files and magnets.txt")
	flag.BoolVar(&Verbose, "v", false, "Verbose - print DBG messages")
	flag.Parse()

	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339})
	if Verbose {
		log.Logger = log.Level(zerolog.DebugLevel)
	} else {
		log.Logger = log.Level(zerolog.InfoLevel)
	}

	if WebDavPath != "" {
		if !strings.HasPrefix(WebDavPath, "/") {
			WebDavPath = "/" + WebDavPath
		}
		if strings.HasSuffix(WebDavPath, "/") {
			log.Fatal().Msg("Shash `/` not allowed at end of secret string")
		}
	} else {
		WebDavPath = "/"
	}

	if ensureDirExists(TorrentsDir) {
		log.Info().Str("Path", TorrentsDir).Msg("New dir for store/watch torrents and magnets.txt")
		os.WriteFile(TorrentsDir+"/stats.txt", []byte("Only for WebDav server"), os.ModePerm)
	}

	TorrentClient = InitTorrentClient()
	Server = NewWebDAVServer(WebDavAddr, WebDavPath)

	//
	initWatcher()
	recursiveScanDir(TorrentsDir)
	log.Info().Int("Count", len(TorrentClient.Torrents())).Msg("Torrents")
	go Server.Run()

	// Ctrl+C
	interrupt := make(chan os.Signal, 1) // we need to reserve to buffer size 1, so the notifier are not blocked
	signal.Notify(interrupt, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-interrupt
		log.Info().Msg("Shutting down...")
		errs := TorrentClient.Close()
		for _, err := range errs {
			log.Error().Err(err).Msg("TorrentClient.Close()")
		}
		PieceCompletion.Close()
		Watcher.Close()
		os.Exit(0)
	}()

	// Write stats on ENTER
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		_ = scanner.Text()
		TorrentClient.WriteStatus(os.Stdout)
	}
	// For non-interactive environment, like containers. Just infinity waiting
	println("AFF")
	time.Sleep(time.Hour * 24 * 365 * 100)
	println("AFF2")
}

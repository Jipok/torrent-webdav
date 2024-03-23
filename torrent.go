package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	torrent_log "github.com/anacrolix/log"
	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/anacrolix/torrent/storage"
	"github.com/inhies/go-bytesize"
	"github.com/rs/zerolog/log"
)

type TorrentWithStorage struct {
	trnt    *torrent.Torrent
	storage *storage.ClientImpl
}

var (
	PieceCompletion storage.PieceCompletion
	// TODO Use sync map
	TorrentStorages map[string]TorrentWithStorage
	TSmu            sync.Mutex
)

type LoggerProxy struct {
}

func (l LoggerProxy) Handle(r torrent_log.Record) {
	log.Info().
		Str("Level", r.Level.LogString()).
		Str("Msg", r.Msg.String()).
		Strs("Names", r.Names).
		Msg("Torrent")
}

func InitTorrentClient() *torrent.Client {
	TorrentStorages = make(map[string]TorrentWithStorage)
	config := torrent.NewDefaultClientConfig()
	//config.Logger = torrent_log.Default.WithFilterLevel(torrent_log.Info)
	config.Seed = true
	config.ListenPort = 4065
	config.Logger.Handlers = append(config.Logger.Handlers, LoggerProxy{})

	// config.HeaderObfuscationPolicy.RequirePreferred = *encrypt

	if ensureDirExists(MetaDataDir) {
		log.Info().Str("Path", MetaDataDir).Msg("New dir for for storing torrents metadata")
	}

	var err error
	PieceCompletion, err = storage.NewSqlitePieceCompletion(MetaDataDir)
	if err != nil {
		log.Fatal().Err(err).Msg("Can't open metadata")
	}

	log.Info().Int("port", config.ListenPort).Msg("Starting torrent client")
	result, err := torrent.NewClient(config)
	if err != nil {
		log.Fatal()
	}
	return result
}

func AddTorrentSpec(spec *torrent.TorrentSpec, path string) *torrent.Torrent {
	spec.Storage = NewMMapWithCompletion(path, PieceCompletion)
	trnt, _, err := TorrentClient.AddTorrentSpec(spec)
	if err != nil {
		log.Error().Str("Path", path).Err(err).Msg("Can't add torrentSpec")
		return nil
	}
	TSmu.Lock()
	TorrentStorages[path] = TorrentWithStorage{
		trnt:    trnt,
		storage: &spec.Storage,
	}
	TSmu.Unlock()

	new := trnt.BytesCompleted() == 0
	if new {
		log.Info().Str("infoHASH", fmt.Sprint(trnt.InfoHash())).Msg("Trying to get torrent MetaInfo")
	}
	<-trnt.GotInfo()

	if new {
		size := bytesize.New(float64(trnt.Length()))
		log.Info().
			Str("Size", size.String()).
			Int("ActivePeers", trnt.Stats().ActivePeers).
			Int("TotalPeers", trnt.Stats().TotalPeers).
			Int("ConnectedSeeders", trnt.Stats().ConnectedSeeders).
			Str("Name", trnt.Name()).
			Msg("New torrent:")
	}

	trnt.DownloadAll()

	prefix, _ := strings.CutPrefix(path, TorrentsDir)
	NewWebDavHandler(trnt, prefix)
	return trnt
}

func AddTorrentFile(path string) {
	mi, err := metainfo.LoadFromFile(path)
	if err != nil {
		log.Error().Str("Path", path).Err(err).Msg("Can't read torrent file")
	}
	ts, err := torrent.TorrentSpecFromMetaInfoErr(mi)
	if err != nil {
		log.Error().Str("Path", path).Err(err).Msg("Can't read torrent specs")
	}
	AddTorrentSpec(ts, filepath.Dir(path))
}

func handleNewTorrentFile(path string) {
	mi, err := metainfo.LoadFromFile(path)
	if err != nil {
		// File can be uploaded partially. Wait
		time.Sleep(time.Second * 3)
		mi, err = metainfo.LoadFromFile(path)
		if err != nil {
			log.Error().Str("Path", path).Err(err).Msg("Can't read torrent file")
			return
		}
	}
	info, err := mi.UnmarshalInfo()
	if err != nil {
		log.Error().Str("Path", path).Err(err).Msg("Can't parse torrent file")
		return
	}
	name, err := storage.ToSafeFilePath(info.Name)
	if err != nil {
		log.Error().Str("Path", path).Str("Name", info.Name).Err(err).Msg("Can't chose correct name for torrent")
		return
	}
	dir := filepath.Dir(path) + "/" + name
	err = os.Mkdir(dir, 0o750)
	if err != nil {
		log.Error().Str("Path", dir).Err(err).Msg("Can't create dir")
		return
	}
	err = os.Rename(path, dir+"/this.torrent")
	if err != nil {
		log.Error().Str("Path", dir).Err(err).Msg("Can't move torrent file")
		return
	}
	log.Info().Str("File", path).Str("Name", name).Msg("Found new torrent")
}

func parseMagnetsFile(path string) {
	buf, err := os.ReadFile(path)
	if err != nil {
		time.Sleep(time.Second * 3)
		buf, err = os.ReadFile(path)
		if err != nil {
			log.Error().Err(err).Str("Path", path).Msg("Can't read file")
			return
		}
	}
	log.Info().Str("Path", path).Msg("Parsing Magnets file")
	for _, line := range strings.Split(string(buf), "\n") {
		trnt, err := TorrentClient.AddMagnet(line)
		if err != nil {
			log.Warn().Err(err).Str("URI", line).Str("File", path).Msg("Can't add Magnet")
			continue
		}
		<-trnt.GotInfo()
		filename := filepath.Dir(path) + "/" + fmt.Sprint(time.Now().UnixNano()) + "-from-Magnet.torrent"
		file, _ := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE, os.ModePerm)
		err = trnt.Metainfo().Write(file)
		if err != nil {
			log.Warn().Err(err).Str("URI", line).Str("File", path).Msg("Can't create torrent file from Magnet")
		}
		trnt.Drop()
		file.Close()
		time.Sleep(time.Nanosecond * 2)
	}
	os.Remove(path)
}

func dropTorrent(path string) {
	TSmu.Lock()
	ts, in := TorrentStorages[path]
	if in {
		ts.trnt.Drop()
		delete(TorrentStorages, path)
		log.Info().Str("Path", path).Msg("Torrent dropped")
	}
	TSmu.Unlock()
}

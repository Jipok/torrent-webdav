package main

import (
	_ "embed"
	"net/http"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Jipok/webdavWithPATCH"
	"github.com/anacrolix/torrent"
	"github.com/rs/zerolog/log"
	"golang.org/x/net/webdav"
)

//go:embed webdavjs.html
var WebdavjsHTML []byte

type handler struct {
	tfs     TFS
	handler *webdavWithPATCH.Handler
}

type WebDAVServer struct {
	addr     string // Address to listen on, e.g. "0.0.0.0:8080"
	smux     *http.ServeMux
	handlers map[string]*handler
	mu       sync.RWMutex
}

func NewWebDAVServer(addr string, secret string) *WebDAVServer {
	WDSrv := WebDAVServer{
		addr: addr,
		smux: http.NewServeMux(),
	}

	WDSrv.handlers = make(map[string]*handler) // URL prefix -> Handler
	mainHandler := &webdavWithPATCH.Handler{
		Handler: webdav.Handler{
			FileSystem: webdav.Dir(TorrentsDir),
			LockSystem: webdav.NewMemLS(),
			Prefix:     secret,
		},
	}

	WDSrv.smux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		if !strings.HasPrefix(req.URL.Path, secret) {
			return
		}

		// Basic Auth
		if Username != "" {
			req_username, req_password, ok := req.BasicAuth()
			log.Debug().
				Str("IP", req.RemoteAddr).
				Str("username", req_username).
				Str("password", req_password).
				Bool("ok", ok).
				Msg("BasicAuth Request")
			if !ok {
				w.Header().Set("WWW-Authenticate", `Basic realm="Restricted"`)
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			if req_username != Username || req_password != Password {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
		}

		method := req.Method
		log.Debug().Str("URL", req.URL.Path).Str("Method", method).Msg("Web Request")

		// Serve fake file
		if (method == "GET" || method == "HEAD") && req.URL.Path == filepath.Join(secret, "stats.txt") {
			TorrentClient.WriteStatus(w)
			return
		}

		// Try find torrent
		WDSrv.mu.RLock()
		for prefix, handler := range WDSrv.handlers {
			println(req.URL.Path)
			println(prefix)
			if !strings.HasPrefix(req.URL.Path, prefix) {
				continue
			}
			// In order not to strain the torrent(which seems to change priorities every
			// NewReader), we send a request to TFS only if the file is not ready
			//
			// Since func returns TRUE if file not exists user can work via WebDav as with
			// a regular file system. The only differences will be when reading unfinished files.
			filename, _ := strings.CutPrefix(req.URL.Path, prefix)
			if !handler.tfs.IsFileCompleted(filename) {
				log.Debug().Str("url", req.URL.Path).Msg("Streaming torrent content")
				handler.ServeHTTP(w, req)
				WDSrv.mu.RUnlock()
				return
			}
			log.Debug().Str("Prefix", prefix).Msg("Access to completed torrent content")
		}
		WDSrv.mu.RUnlock()

		// Work as a WebDav or Web server for a regular file system

		if method == "GET" && strings.HasSuffix(req.URL.Path, "/") {
			if _, err := w.Write(WebdavjsHTML); err != nil {
				log.Error().Err(err).Msg("Failed to write index.html")
			}
			return
		}

		// Fix web ui
		if method == "HEAD" && strings.HasSuffix(req.URL.Path, "/") {
			return
		}

		mainHandler.ServeHTTP(w, req)
	})

	return &WDSrv
}

func (s *WebDAVServer) Run() {
	log.Info().Str("addr", "http://"+s.addr+WebDavPath).Msg("WebDAV server started")
	if err := http.ListenAndServe(s.addr, s.smux); err != nil {
		panic(err)
	}
}

/////////////////////////////////////////////////////////////////////////////////

func NewWebDavHandler(trnt *torrent.Torrent, prefix string) {
	prefix = filepath.Join(WebDavPath, prefix)
	log.Debug().Str("Prefix", prefix).Msg("New WebDav Handler")
	tfs := *NewTFS(trnt)
	handler := &handler{
		tfs: tfs,
		handler: &webdavWithPATCH.Handler{
			Handler: webdav.Handler{
				FileSystem: tfs,
				LockSystem: webdav.NewMemLS(),
				Prefix:     prefix,
			},
		},
	}
	Server.mu.Lock()
	Server.handlers[prefix] = handler
	Server.mu.Unlock()
}

func (h *handler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	allowedMethods := map[string]bool{
		"GET":      true,
		"OPTIONS":  true,
		"PROPFIND": true,
		"HEAD":     true,
	}
	if !allowedMethods[req.Method] {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if req.Method == "GET" && strings.HasSuffix(req.URL.Path, "/") {
		if _, err := w.Write(WebdavjsHTML); err != nil {
			log.Error().Err(err).Msg("Failed to write index.html")
		}
		return
	}

	// Fix web ui
	if req.Method == "HEAD" && strings.HasSuffix(req.URL.Path, "/") {
		return
	}

	h.handler.ServeHTTP(w, req)
}

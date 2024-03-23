package main

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/rs/zerolog/log"
	"golang.org/x/net/webdav"
)

type TFS struct {
	torrent *torrent.Torrent
	list    map[string]*TFS_File
	root    *TFS_File
}

type TFS_File struct {
	t_file   *torrent.File
	children []*TFS_File
	name     string
	size     int64
	mode     os.FileMode
	modTime  time.Time
}

type TFS_FileHandler struct {
	fileOrDir *TFS_File
	mu        sync.Mutex
	reader    torrent.Reader
	closed    bool
}

func NewTFS(torrent *torrent.Torrent) *TFS {
	tfs := &TFS{}
	tfs.torrent = torrent
	modTime := time.Unix(torrent.Metainfo().CreationDate, 0)
	tfs.list = make(map[string]*TFS_File)
	tfs.root = &TFS_File{
		name:    "/",
		mode:    0660 | os.ModeDir,
		modTime: modTime,
	}
	tfs.list["/"] = tfs.root

	for _, f := range torrent.Files() {
		// Try find all parents dirs, add if not exists
		// dir1/dir2/dir3/file   ->   dir1,  dir1/dir2,  dir1/dir2/dir3
		log.Debug().Str("file", f.DisplayPath()).Msg("TFS")
		parts := strings.Split(f.DisplayPath(), "/")
		parentDir := tfs.root
		path := ""
		for _, dir := range parts[:len(parts)-1] {
			path = filepath.Join(path, dir)
			tmp, in := tfs.list[path]
			if !in {
				tmp = &TFS_File{
					t_file:  nil,
					name:    dir,
					size:    0,
					mode:    0555 | os.ModeDir,
					modTime: modTime,
				}
				tfs.list["/"+path] = tmp
				parentDir.children = append(parentDir.children, tmp)
			}
			parentDir = tmp
		}
		// We assume that there are no empty folders and each path is a file
		file := TFS_File{
			t_file:  f,
			name:    parts[len(parts)-1],
			size:    f.Length(),
			mode:    0555,
			modTime: modTime,
		}
		parentDir.children = append(parentDir.children, &file)
		tfs.list["/"+f.DisplayPath()] = &file
	}
	return tfs
}

// Return TRUE for non-existent files.
func (tfs TFS) IsFileCompleted(name string) bool {
	entry, found := tfs.list[name]
	if !found {
		return true
	}
	if entry.mode.IsDir() {
		return true
	}
	len := entry.t_file.Length()
	bc := entry.t_file.BytesCompleted()
	if bc > len {
		println("!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!")
		log.Warn().Int64("Length", len).Int64("BytesCompleted", bc).Msg("WTF TODO") //TODO
	}
	return bc >= len
}

////////// FileSystem interface

func (tfs TFS) Mkdir(ctx context.Context, name string, perm os.FileMode) error {
	return fs.ErrPermission
}

func (tfs TFS) RemoveAll(ctx context.Context, name string) error {
	return fs.ErrPermission
}

func (tfs TFS) Rename(ctx context.Context, oldName, newName string) error {
	return fs.ErrPermission
}

func (tfs TFS) Stat(ctx context.Context, name string) (os.FileInfo, error) {
	entry, found := tfs.list[name]
	if found {
		return entry, nil
	}
	return nil, fs.ErrNotExist
}

func (tfs TFS) OpenFile(ctx context.Context, name string, flag int, perm os.FileMode) (webdav.File, error) {
	entry, found := tfs.list[name]
	if !found {
		return nil, fs.ErrNotExist
	}
	// We do not create a reader there, since webdav calls OpenFile twice for each file with PROPFIND
	// Then it closes immediately. And creating a reader is a costly operation
	handler := TFS_FileHandler{
		fileOrDir: entry,
	}
	return &handler, nil
}

////////// fs.FileInfo interface

func (f *TFS_File) Name() string       { return f.name }
func (f *TFS_File) Size() int64        { return f.size }
func (f *TFS_File) Mode() os.FileMode  { return f.mode }
func (f *TFS_File) ModTime() time.Time { return f.modTime }
func (f *TFS_File) IsDir() bool        { return f.mode.IsDir() }
func (f *TFS_File) Sys() interface{}   { return nil }

////////// WebDav.File interface

// io.Writer
func (f *TFS_FileHandler) Write(p []byte) (n int, err error) {
	return 0, fs.ErrPermission
}

// io.Closer
func (f *TFS_FileHandler) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return fs.ErrClosed
	}
	if f.reader != nil {
		f.reader.Close()
		f.closed = true
	}
	return nil
}

// io.Reader
func (f *TFS_FileHandler) Read(p []byte) (n int, err error) {
	if f.fileOrDir.IsDir() {
		return 0, os.ErrInvalid
	}
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.closed {
		return 0, os.ErrClosed
	}
	if f.reader == nil {
		f.reader = f.fileOrDir.t_file.NewReader()
	}
	return f.reader.Read(p)
}

// io.Seeker
func (f *TFS_FileHandler) Seek(offset int64, whence int) (int64, error) {
	if f.fileOrDir.IsDir() {
		return 0, os.ErrInvalid
	}
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.closed {
		return 0, os.ErrClosed
	}
	if f.reader == nil {
		f.reader = f.fileOrDir.t_file.NewReader()
	}
	return f.reader.Seek(offset, whence)
}

// http.File
func (f *TFS_FileHandler) Stat() (fs.FileInfo, error) {
	return f.fileOrDir, nil
}

// http.File
func (f *TFS_FileHandler) Readdir(count int) ([]fs.FileInfo, error) {
	if !f.fileOrDir.IsDir() {
		return nil, os.ErrInvalid
	}
	f.mu.Lock()
	defer f.mu.Unlock()

	// Seems Readdir is only called in webdav/file.go: walkFS()
	// Always with count = 0
	// So we simplify the implementation and return all values

	// Convert []*TFS_File  to  []fs.FileInfo
	var fileInfoList []fs.FileInfo
	for _, entry := range f.fileOrDir.children {
		fileInfoList = append(fileInfoList, entry)
	}
	return fileInfoList, nil
}

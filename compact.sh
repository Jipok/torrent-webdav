#!/bin/sh
go build -ldflags "-s -w" && upx trnt2webdav

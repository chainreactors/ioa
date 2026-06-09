//go:build sqlite

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/chainreactors/ioa/server"
	"github.com/chainreactors/ioa/sqlite"
)

func init() {
	openStore = openSQLiteStore
}

func openSQLiteStore(opts options) (server.Store, func() error, string, error) {
	dbPath := opts.DB
	if dbPath == "" {
		dbPath = "./ioa.db"
	}
	if !filepath.IsAbs(dbPath) {
		if wd, err := os.Getwd(); err == nil {
			dbPath = filepath.Join(wd, dbPath)
		}
	}
	store, err := sqlite.NewSQLiteStore(dbPath)
	if err != nil {
		return nil, nil, "", fmt.Errorf("open database: %s", err)
	}
	return store, store.Close, "sqlite:" + dbPath, nil
}

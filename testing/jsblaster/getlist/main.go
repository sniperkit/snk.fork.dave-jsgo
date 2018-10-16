/*
Sniperkit-Bot
- Status: analyzed
*/

package main

import (
	"context"
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"cloud.google.com/go/datastore"
	"github.com/dave/patsy"
	"github.com/dave/patsy/vos"
	"google.golang.org/api/iterator"

	"github.com/sniperkit/snk.fork.dave-jsgo/config"
	"github.com/sniperkit/snk.fork.dave-jsgo/server/store"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	ctx := context.Background()
	client, err := datastore.NewClient(ctx, config.ProjectID)
	if err != nil {
		return err
	}
	defer client.Close()

	dir, err := patsy.Dir(vos.Os(), "github.com/sniperkit/snk.fork.dave-jsgo/testing/jsblaster")
	if err != nil {
		return err
	}

	file, err := os.Create(filepath.Join(dir, "packages.csv"))
	if err != nil {
		return err
	}
	defer file.Close()
	w := csv.NewWriter(file)
	if err := w.Write([]string{"package"}); err != nil {
		return err
	}

	q := datastore.NewQuery(config.PackageKind)
	it := client.Run(ctx, q)

	for {
		var data store.CompileData
		_, err := it.Next(&data)
		if err != nil {
			if err == iterator.Done {
				break
			}
			return err
		}
		fmt.Println(data.Path)
		if err := w.Write([]string{data.Path}); err != nil {
			return err
		}
	}

	w.Flush()

	return nil
}

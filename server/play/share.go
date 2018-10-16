/*
Sniperkit-Bot
- Status: analyzed
*/

package play

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"cloud.google.com/go/storage"
	"github.com/dave/play/models"
	"github.com/dave/services"
	"github.com/dave/services/constor"
	"github.com/dave/services/constor/constormsg"

	"github.com/sniperkit/snk.fork.dave-jsgo/config"
	"github.com/sniperkit/snk.fork.dave-jsgo/server/play/messages"
	"github.com/sniperkit/snk.fork.dave-jsgo/server/store"
)

func (h *Handler) Share(ctx context.Context, info messages.Share, req *http.Request, send func(message services.Message), receive chan services.Message) error {

	send(constormsg.Storing{Starting: true})

	sp := models.SharePack{
		Version: 0,
		Source:  info.Source,
		Tags:    info.Tags,
	}

	buf := &bytes.Buffer{}
	sha := sha1.New()
	w := io.MultiWriter(buf, sha)
	if err := json.NewEncoder(w).Encode(sp); err != nil {
		return err
	}
	hash := sha.Sum(nil)

	client, err := storage.NewClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	storer := constor.New(ctx, h.Fileserver, send, config.ConcurrentStorageUploads)
	storer.Add(constor.Item{
		Message:   "source",
		Name:      fmt.Sprintf("%x.json", hash),
		Contents:  buf.Bytes(),
		Bucket:    config.Bucket[config.Src],
		Mime:      constor.MimeJson,
		Count:     true,
		Immutable: true,
		Send:      true,
	})
	if err := storer.Wait(); err != nil {
		return err
	}

	send(constormsg.Storing{Done: true})

	if err := h.storeShare(ctx, info.Source, fmt.Sprintf("%x", hash), send, req); err != nil {
		return err
	}

	send(messages.ShareComplete{Hash: fmt.Sprintf("%x", hash)})

	return nil
}

func (h *Handler) storeShare(ctx context.Context, source map[string]map[string]string, hash string, send func(services.Message), req *http.Request) error {
	var count int
	for _, pkg := range source {
		for range pkg {
			count++
		}
	}
	data := store.ShareData{
		Time:  time.Now(),
		Ip:    req.Header.Get("X-Forwarded-For"),
		Files: count,
		Hash:  hash,
	}
	if err := store.StoreShare(ctx, h.Database, data); err != nil {
		return err
	}
	return nil
}

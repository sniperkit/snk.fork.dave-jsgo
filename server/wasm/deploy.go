/*
Sniperkit-Bot
- Status: analyzed
*/

package wasm

import (
	"bytes"
	"context"
	"crypto/sha1"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/dave/services"
	"github.com/dave/services/constor"
	"github.com/dave/services/constor/constormsg"

	"github.com/sniperkit/snk.fork.dave-jsgo/config"
	"github.com/sniperkit/snk.fork.dave-jsgo/server/servermsg"
	"github.com/sniperkit/snk.fork.dave-jsgo/server/store"
	"github.com/sniperkit/snk.fork.dave-jsgo/server/wasm/messages"
)

func (h *Handler) DeployQuery(ctx context.Context, info messages.DeployQuery, req *http.Request, send func(services.Message), receive chan services.Message) error {

	var m sync.Mutex
	var required []messages.DeployFileKey
	wg := &sync.WaitGroup{}

	for _, file := range info.Files {
		file := file
		var outer error
		wg.Add(1)
		go func() {
			defer wg.Done()
			bucket, name, _ := details(file.Type, file.Hash)
			exists, err := h.Fileserver.Exists(ctx, bucket, name)
			if err != nil {
				outer = err
				return
			}
			if !exists {
				m.Lock()
				required = append(required, file)
				m.Unlock()
			}
		}()
	}
	wg.Wait()

	send(messages.DeployQueryResponse{Required: required})

	if len(required) == 0 {
		return nil
	}

	var payload messages.DeployPayload
	select {
	case message := <-receive:
		payload = message.(messages.DeployPayload)
	case <-ctx.Done():
		return nil
	}

	storer := constor.New(ctx, h.Fileserver, send, config.ConcurrentStorageUploads)
	defer storer.Close()

	var files []store.WasmDeployFile
	for _, f := range payload.Files {
		files = append(files, store.WasmDeployFile{Type: string(f.Type), Hash: f.Hash})
		// check the hash is correct
		sha := sha1.New()
		if _, err := io.Copy(sha, bytes.NewBuffer(f.Contents)); err != nil {
			return err
		}
		calculated := fmt.Sprintf("%x", sha.Sum(nil))
		if calculated != f.Hash {
			return fmt.Errorf("hash not consistent for %s", f.Type)
		}
		bucket, name, mime := details(f.Type, f.Hash)
		storer.Add(constor.Item{
			Message:   string(f.Type),
			Name:      name,
			Contents:  f.Contents,
			Bucket:    bucket,
			Mime:      mime,
			Count:     true,
			Immutable: true,
			Send:      true,
		})
	}

	if err := storer.Wait(); err != nil {
		return err
	}

	send(constormsg.Storing{Done: true})

	h.storeWasmDeploy(ctx, send, req, files)

	send(messages.DeployDone{})

	return nil
}

func (h *Handler) storeWasmDeploy(ctx context.Context, send func(services.Message), req *http.Request, files []store.WasmDeployFile) {
	data := store.WasmDeploy{
		Time:  time.Now(),
		Ip:    req.Header.Get("X-Forwarded-For"),
		Files: files,
	}
	if err := store.StoreWasmDeploy(ctx, h.Database, data); err != nil {
		// don't save this one to the datastore because it's an error from the datastore.
		send(servermsg.Error{Message: err.Error()})
		return
	}
}

func details(typ messages.DeployFileType, hash string) (bucket, name, mime string) {
	switch typ {
	case messages.DeployFileTypeIndex:
		bucket = config.Bucket[config.Index]
		name = hash
		mime = constor.MimeHtml
	case messages.DeployFileTypeLoader:
		bucket = config.Bucket[config.Pkg]
		name = fmt.Sprintf("%s.js", hash)
		mime = constor.MimeJs
	case messages.DeployFileTypeWasm:
		bucket = config.Bucket[config.Pkg]
		name = fmt.Sprintf("%s.wasm", hash)
		mime = constor.MimeWasm
	}
	return
}

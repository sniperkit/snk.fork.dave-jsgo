/*
Sniperkit-Bot
- Status: analyzed
*/

package play

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/dave/services"
	"github.com/dave/services/getter/get"
	"github.com/dave/services/getter/gettermsg"
	"github.com/dave/services/session"
	"github.com/shurcooL/go/ctxhttp"
	"gopkg.in/src-d/go-billy.v4"

	"github.com/sniperkit/snk.fork.dave-jsgo/assets"
	"github.com/sniperkit/snk.fork.dave-jsgo/config"
	"github.com/sniperkit/snk.fork.dave-jsgo/server/play/messages"
)

func (h *Handler) Get(ctx context.Context, info messages.Get, req *http.Request, send func(message services.Message), receive chan services.Message) error {
	s := session.New(nil, assets.Assets, assets.Archives, h.Fileserver, config.ValidExtensions)
	g := get.New(s, send, h.Cache.NewRequest(false))
	_, err := getSource(ctx, g, s, info.Path, send)
	if err != nil {
		return err
	}
	return nil
}

func getSource(ctx context.Context, g *get.Getter, s *session.Session, path string, send func(message services.Message)) (map[string]map[string]string, error) {

	if strings.HasPrefix(path, "p/") {
		send(gettermsg.Downloading{Message: path})
		source, err := getGolangPlaygroundSource(ctx, path)
		if err != nil {
			return nil, err
		}
		send(gettermsg.Downloading{Done: true})
		send(messages.GetComplete{Source: source})
		return source, nil
	}

	root := filepath.Join("goroot", "src", path)
	if _, err := assets.Assets.Stat(root); err == nil {
		// Look in the goroot for standard lib packages
		source, err := getSourceFiles(assets.Assets, path, root)
		if err != nil {
			return nil, err
		}
		send(messages.GetComplete{Source: source})
		return source, nil
	}

	// Send a message to the client that downloading step has started.
	send(gettermsg.Downloading{Starting: true})

	// set insecure = true in local mode or it will fail if git repo has git protocol
	insecure := config.LOCAL

	// Start the download process - just like the "go get" command.
	// Don't need to give git hints here because only one package will be downloaded
	if err := g.Get(ctx, path, false, insecure, true); err != nil {
		return nil, err
	}

	source, err := getSourceFiles(s.GoPath(), path, filepath.Join("gopath", "src", path))
	if err != nil {
		return nil, err
	}

	// Send a message to the client that downloading step has finished.
	send(gettermsg.Downloading{Done: true})
	send(messages.GetComplete{Source: source})

	return source, nil
}

func getSourceFiles(fs billy.Filesystem, path, dir string) (map[string]map[string]string, error) {
	source := map[string]map[string]string{}
	fis, err := fs.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, fi := range fis {
		if !isValidFile(fi.Name()) {
			continue
		}
		if strings.HasSuffix(fi.Name(), "_test.go") {
			continue
		}
		f, err := fs.Open(filepath.Join(dir, fi.Name()))
		if err != nil {
			return nil, err
		}
		b, err := ioutil.ReadAll(f)
		if err != nil {
			f.Close()
			return nil, err
		}
		f.Close()
		if source[path] == nil {
			source[path] = map[string]string{}
		}
		source[path][fi.Name()] = string(b)
	}
	return source, nil
}

func isValidFile(name string) bool {
	for _, ext := range config.ValidExtensions {
		if strings.HasSuffix(name, ext) {
			return true
		}
	}
	return false
}

func getGolangPlaygroundSource(ctx context.Context, path string) (map[string]map[string]string, error) {
	var httpClient = &http.Client{
		Timeout: config.HttpTimeout,
	}
	resp, err := ctxhttp.Get(ctx, httpClient, fmt.Sprintf("https://play.golang.org/%s.go", path))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("error %d", resp.StatusCode)
	}
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	source := map[string]map[string]string{
		path: {
			"main.go": string(b),
		},
	}
	return source, nil
}

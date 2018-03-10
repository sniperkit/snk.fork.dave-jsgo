package server

import (
	"bytes"
	"fmt"
	"html/template"
	"io"
	"mime"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	pathpkg "path"

	"errors"

	"regexp"

	"context"

	"sync"

	"github.com/dave/jsgo/assets"
	"github.com/dave/jsgo/builder/std"
	"github.com/dave/jsgo/config"
	"github.com/dave/jsgo/getter"
	"github.com/dave/jsgo/server/compile"
	"github.com/dave/jsgo/server/messages"
	"github.com/dave/jsgo/server/queue"
	"github.com/dave/jsgo/server/store"
	"github.com/dustin/go-humanize"
	"github.com/gorilla/websocket"
	"github.com/shurcooL/httpgzip"
	"gopkg.in/src-d/go-billy.v4"
	"gopkg.in/src-d/go-billy.v4/memfs"
)

func New(shutdown chan struct{}) *Handler {
	h := &Handler{
		mux:       http.NewServeMux(),
		shutdown:  shutdown,
		Queue:     queue.New(config.MaxConcurrentCompiles, config.MaxQueue),
		Waitgroup: &sync.WaitGroup{},
	}
	h.mux.HandleFunc("/", h.PageHandler)
	//h.mux.HandleFunc("/_foo/", h.Foo)
	h.mux.HandleFunc("/_ws/", h.SocketHandler)
	h.mux.HandleFunc("/_pg/", h.SocketHandler)
	h.mux.HandleFunc("/favicon.ico", h.IconHandler)
	h.mux.HandleFunc("/compile.css", h.CssHandler)
	h.mux.HandleFunc("/_ah/health", h.HealthCheckHandler)
	h.mux.HandleFunc("/_go", h.GoCheckHandler)
	return h
}

func (h *Handler) Foo(w http.ResponseWriter, req *http.Request) {

	// creates the in-cluster config
	cfg, err := rest.InClusterConfig()
	if err != nil {
		http.Error(w, err.Error(), 500)
	}

	// creates the clientset
	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		http.Error(w, err.Error(), 500)
	}

	pods, err := clientset.CoreV1().Pods("").List(metav1.ListOptions{})
	if err != nil {
		http.Error(w, err.Error(), 500)
	}
	fmt.Fprintf(w, "There are %d pods in the cluster:\n\n", len(pods.Items))

	for k, v := range pods.Items {
		fmt.Fprintf(w, "Pod %d:\n", k)
		fmt.Fprintf(w, "TypeMeta: %#v\n", v.TypeMeta)
		fmt.Fprintf(w, "ObjectMeta: %#v\n", v.ObjectMeta)
		fmt.Fprintf(w, "Spec: %#v\n", v.Spec)
		fmt.Fprintf(w, "Status: %#v\n", v.Status)
		fmt.Fprintln(w)
	}

	// Examples for error handling:
	// - Use helper functions like e.g. errors.IsNotFound()
	// - And/or cast to StatusError and use its properties like e.g. ErrStatus.Message
	/*
		_, err = clientset.CoreV1().Pods("default").Get("example-xxxxx", metav1.GetOptions{})
		if kerrors.IsNotFound(err) {
			fmt.Printf("Pod not found\n")
		} else if statusError, isStatus := err.(*kerrors.StatusError); isStatus {
			fmt.Printf("Error getting pod %v\n", statusError.ErrStatus.Message)
		} else if err != nil {
			panic(err.Error())
		} else {
			fmt.Printf("Found pod\n")
		}

		time.Sleep(10 * time.Second)
	*/

}

type Handler struct {
	Waitgroup *sync.WaitGroup
	Queue     *queue.Queue
	mux       *http.ServeMux
	shutdown  chan struct{}
}

func (h *Handler) PageHandler(w http.ResponseWriter, req *http.Request) {

	ctx, cancel := context.WithTimeout(req.Context(), config.PageTimeout)
	defer cancel()

	path := normalizePath(strings.TrimSuffix(strings.TrimPrefix(req.URL.Path, "/"), "/"))

	if path == "" {
		http.Redirect(w, req, "https://github.com/dave/jsgo", http.StatusFound)
		return
	}

	found, data, err := store.Lookup(ctx, path)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	type vars struct {
		Found     bool
		Path      string
		Last      string
		Host      string
		Scheme    string
		PkgHost   string
		IndexHost string
	}

	v := vars{}
	v.PkgHost = config.PkgHost
	v.IndexHost = config.IndexHost
	v.Host = req.Host
	v.Path = path
	if req.Host == config.CompileHost {
		v.Scheme = "wss"
	} else {
		v.Scheme = "ws"
	}
	if found {
		v.Found = true
		v.Last = humanize.Time(data.Time)
	}

	if err := pageTemplate.Execute(w, v); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
}

var pageTemplate = template.Must(template.New("main").Parse(`
<html>
	<head>
		<meta charset="utf-8">
		<link href="https://maxcdn.bootstrapcdn.com/bootstrap/4.0.0/css/bootstrap.min.css" rel="stylesheet" integrity="sha384-Gn5384xqQ1aoWXA+058RXPxPg6fy4IWvTNh0E263XmFcJlSAwiGgFAW/dAiS6JXm" crossorigin="anonymous">
		<link href="/compile.css" rel="stylesheet">
	</head>
	<body>
		<div class="site-wrapper">
			<div class="site-wrapper-inner">
				<div class="cover-container">
					<div class="masthead clearfix">
						<div class="inner">
							<h3 class="masthead-brand">jsgo</h3>
							<nav class="nav nav-masthead">
								<a class="nav-link active" href="">Compile</a>
								<a class="nav-link" href="https://github.com/dave/jsgo">Info</a>
							</nav>
						</div>
					</div>

					<div id="header-panel" class="inner cover">
						<h1 class="cover-heading">Compile</h1>
						<p class="lead">
							{{ .Path }}
							{{ if .Found }} was compiled {{ .Last }} {{ end }}
						</p>
						<p class="lead" id="button-panel">
							<a href="#" class="btn btn-lg btn-secondary" id="btn">Compile</a>
						</p>
					</div>

					<div id="complete-panel" style="display: none;">
						<div class="inner cover">
							<h1 class="cover-heading">
								Complete!
							</h1>

							<h3><small class="text-muted">Link</small></h3>
							<p>
								<a id="complete-link" href=""></a>
							</p>

							<h3><small class="text-muted">Loader JS</small></h3>
							<p>
								<input id="complete-script" type="text" onclick="this.select()" class="form-control" />
							</p>

							<p>
								<small>
									<input type="checkbox" id="minify-checkbox" checked> <label for="minify-checkbox" class="text-muted">Minify</label>
								</small>
								<small id="short-url-checkbox-holder">
									<input type="checkbox" id="short-url-checkbox" checked> <label for="short-url-checkbox" class="text-muted">Short URL</label>
								</small>
							</p>
							
						</div>
					</div>

					<div id="progress-panel" style="display: none;">
						<table class="table table-dark">
							<tbody>
								<tr id="queue-item" style="display: none;">
									<th scope="row" class="w-25">Queued:</th>
									<td class="w-75"><span id="queue-span"></span></td>
								</tr>
								<tr id="download-item" style="display: none;">
									<th scope="row" class="w-25">Downloading:</th>
									<td class="w-75"><span id="download-span"></span></td>
								</tr>
								<tr id="compile-item" style="display: none;">
									<th scope="row" class="w-25">Compiling:</th>
									<td class="w-75"><span id="compile-span"></span></td>
								</tr>
								<tr id="store-item" style="display: none;">
									<th scope="row" class="w-25">Storing:</th>
									<td class="w-75"><span id="store-span"></span></td>
								</tr>
							</tbody>
						</table>
					</div>
					<div id="error-panel" style="display: none;" class="alert alert-warning" role="alert">
						<h4 class="alert-heading">Error</h4>
						<pre id="error-message"></pre>
					</div>
				</div>
			</div>
		</div>
	</body>
	<script>
		var final = {};
		var refresh = function() {
			var minify = document.getElementById("minify-checkbox").checked;
			var short = document.getElementById("short-url-checkbox").checked;
			var completeLink = document.getElementById("complete-link");
			var completeScript = document.getElementById("complete-script");
			var shortUrlCheckboxHolder = document.getElementById("short-url-checkbox-holder");
			
			shortUrlCheckboxHolder.style.display = (final.short == final.path) ? "none" : "";
			completeLink.href = "https://{{ .IndexHost }}/" + (short ? final.short : final.path) + (minify ? "" : "$max");
			completeLink.innerHTML = "{{ .IndexHost }}/" + (short ? final.short : final.path) + (minify ? "" : "$max");
			completeScript.value = "https://{{ .PkgHost }}/" + final.path + "." + (minify ? final.hashmin : final.hashmax) + ".js"
		}
		document.getElementById("minify-checkbox").onchange = refresh;
		document.getElementById("short-url-checkbox").onchange = refresh;
		document.getElementById("btn").onclick = function(event) {
			event.preventDefault();
			var socket = new WebSocket("{{ .Scheme }}://{{ .Host }}/_ws/{{ .Path }}");

			var headerPanel = document.getElementById("header-panel");
			var buttonPanel = document.getElementById("button-panel");
			var progressPanel = document.getElementById("progress-panel");
			var errorPanel = document.getElementById("error-panel");
			var completePanel = document.getElementById("complete-panel");
			var errorMessage = document.getElementById("error-message");
			
			var done = {};
			var complete = false;

			socket.onopen = function() {
				buttonPanel.style.display = "none";
				progressPanel.style.display = "";
			};
			socket.onmessage = function (e) {
				var payload = JSON.parse(e.data)
				switch (payload.type) {
				case "Queue":
				case "Download":
				case "Compile":
				case "Store":
					if (done[payload.type]) {
						// Messages might arrive out of order... Once we get a "done", ignore 
						// any more.
						break;
					}
					var item = document.getElementById(payload.type.toLowerCase()+"-item");
					var span = document.getElementById(payload.type.toLowerCase()+"-span");
					item.style.display = "";
					if (payload.message.done) {
						span.innerHTML = "Done";
						done[payload.type] = true;
					} else if (payload.message.starting) {
						span.innerHTML = "Starting";
					} else if (payload.message.message) {
						span.innerHTML = payload.message.message;
					} else if (payload.message.position) {
						span.innerHTML = "Position " + payload.message.position;
					} else if (payload.message.finished !== undefined) {
						span.innerHTML = payload.message.finished + " finished, " + payload.message.unchanged + " unchanged, " + payload.message.remain + " remain.";
					} else {
						span.innerHTML = "Starting";
					}
					break;
				case "Complete":
					complete = true;
					final = payload.message;
					completePanel.style.display = "";
					progressPanel.style.display = "none";
					headerPanel.style.display = "none";
					refresh();
					break;
				case "Error":
					if (complete) {
						break;
					}
					complete = true;
					errorPanel.style.display = "";
					errorMessage.innerHTML = payload.message.message;
					break;
				}
				socket.onclose = function() {
					if (complete) {
						return;
					}
					errorPanel.style.display = "";
					errorMessage.innerHTML = "server disconnected";
				}
			}
		};
	</script>
</html>
`))

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

const (
	TypeCompile = iota
	TypePlaygroundCompile
	TypePlaygroundDeploy
)

func (h *Handler) SocketHandler(w http.ResponseWriter, req *http.Request) {

	h.Waitgroup.Add(1)
	defer h.Waitgroup.Done()

	ctx, cancel := context.WithTimeout(req.Context(), config.CompileTimeout)
	defer cancel()

	var compileType int
	var prefix string

	if strings.HasPrefix(req.URL.Path, "/_pg/") {
		compileType = TypePlaygroundCompile
		prefix = "/_pg/"
	} else {
		compileType = TypeCompile
		prefix = "/_ws/"
	}

	path := normalizePath(strings.TrimSuffix(strings.TrimPrefix(req.URL.Path, prefix), "/"))

	conn, err := upgrader.Upgrade(w, req, nil)
	if err != nil {
		storeError(ctx, path, fmt.Errorf("upgrading request to websocket: %v", err), req)
		return
	}
	defer func() {
		// wait for sends to finish before closing websocket
		// TODO: Find better way of doing this
		<-time.After(time.Millisecond * 200)
		conn.Close()
	}()

	send := make(chan messages.Message, 256)
	receive := make(chan messages.Message, 256)

	// Recover from any panic and log the error.
	defer func() {
		if r := recover(); r != nil {
			sendAndStoreError(ctx, send, path, errors.New(fmt.Sprintf("panic recovered: %s", r)), req)
		}
	}()

	// Set up a ticker to ping the client regularly
	go func() {
		ticker := time.NewTicker(config.WebsocketPingPeriod)
		defer func() {
			ticker.Stop()
			cancel()
		}()
		for {
			select {
			case message, ok := <-send:
				conn.SetWriteDeadline(time.Now().Add(config.WebsocketWriteTimeout))
				if !ok {
					// The send channel was closed.
					conn.WriteMessage(websocket.CloseMessage, []byte{})
					return
				}
				b, err := messages.Marshal(message)
				if err != nil {
					conn.WriteMessage(websocket.CloseMessage, []byte{})
					return
				}
				if err := conn.WriteMessage(websocket.TextMessage, b); err != nil {
					// Error writing message, close and exit
					conn.WriteMessage(websocket.CloseMessage, []byte{})
					return
				}
			case <-ticker.C:
				conn.SetWriteDeadline(time.Now().Add(config.WebsocketWriteTimeout))
				if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// React to pongs from the client
	go func() {
		defer func() {
			cancel()
		}()
		conn.SetReadDeadline(time.Now().Add(config.WebsocketPongTimeout))
		conn.SetPongHandler(func(string) error {
			conn.SetReadDeadline(time.Now().Add(config.WebsocketPongTimeout))
			return nil
		})
		for {
			_, messageBytes, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
					// Don't bother storing an error if the client disconnects gracefully
					break
				}
				storeError(ctx, path, err, req)
				break
			}
			message, err := messages.Unmarshal(messageBytes)
			if err != nil {
				storeError(ctx, path, err, req)
				break
			}
			select {
			case receive <- message:
			default:
			}
		}
	}()

	// React to the server shutdown signal
	go func() {
		select {
		case <-h.shutdown:
			sendAndStoreError(ctx, send, path, errors.New("server shut down"), req)
			cancel()
		case <-ctx.Done():
		}
	}()

	// Request a slot in the queue...
	start, end, err := h.Queue.Slot(func(position int) {
		send <- messages.Queue{Position: position}
	})
	if err != nil {
		sendAndStoreError(ctx, send, path, err, req)
		return
	}

	// Signal to the queue that processing has finished.
	defer close(end)

	// Wait for the slot to become available.
	select {
	case <-start:
		// continue
	case <-ctx.Done():
		return
	}

	// Send a message to the client that queue step has finished.
	send <- messages.Queue{Done: true}

	switch compileType {
	case TypeCompile:
		doCompile(ctx, path, req, send)
	case TypePlaygroundCompile:
		doPlaygroundCompile(ctx, path, req, send, receive)
	}

	return
}

func doPlaygroundCompile(ctx context.Context, path string, req *http.Request, send, receive chan messages.Message) {

	var message messages.Message
	select {
	case message = <-receive:
		// got initial message
	case <-time.After(config.WebsocketInstructionTimeout):
		sendAndStoreError(ctx, send, path, errors.New("timed out waiting for instruction from client"), req)
	}

	switch message := message.(type) {
	case messages.PlaygroundCompile:
		// TODO
		fmt.Println(message)
		send <- messages.Complete{Path: path}
	default:
		// TODO
	}
}

func doCompile(ctx context.Context, path string, req *http.Request, send chan messages.Message) {
	// Create a memory filesystem for the getter to store downloaded files (e.g. GOPATH).
	fs := memfs.New()

	// Send a message to the client that downloading step has started.
	send <- messages.Download{Starting: true}

	// Start the download process - just like the "go get" command.
	if err := getter.New(fs, messages.DownloadWriter(send), []string{"jsgo"}).Get(ctx, path, false, false); err != nil {
		sendAndStoreError(ctx, send, path, err, req)
		return
	}

	// Send a message to the client that downloading step has finished.
	send <- messages.Download{Done: true}

	// Start the compile process - this compiles to JS and sends the files to a GCS bucket.
	min, max, err := compile.New(assets.Assets, fs, send).Compile(ctx, path)
	if err != nil {
		sendAndStoreError(ctx, send, path, err, req)
		return
	}

	// Logs the success in the datastore
	storeSuccess(ctx, send, path, req, min, max)

	// Send a message to the client that the process has successfully finished
	send <- messages.Complete{
		Path:    path,
		Short:   strings.TrimPrefix(path, "github.com/"),
		HashMin: fmt.Sprintf("%x", min.Hash),
		HashMax: fmt.Sprintf("%x", max.Hash),
	}
}

func storeSuccess(ctx context.Context, send chan messages.Message, path string, req *http.Request, min, max *compile.CompileOutput) {
	getCompileContents := func(c *compile.CompileOutput, min bool) store.CompileContents {
		val := store.CompileContents{}
		val.Main = fmt.Sprintf("%x", c.Hash)
		var preludeHash string
		if min {
			preludeHash = std.PreludeMin
		} else {
			preludeHash = std.PreludeMax
		}
		val.Packages = []store.CompilePackage{
			{
				Path:     "prelude",
				Hash:     preludeHash,
				Standard: true,
			},
		}
		for _, p := range c.Packages {
			val.Packages = append(val.Packages, store.CompilePackage{
				Path:     p.Path,
				Hash:     fmt.Sprintf("%x", p.Hash),
				Standard: p.Standard,
			})
		}
		return val
	}

	data := store.CompileData{
		Path:    path,
		Time:    time.Now(),
		Success: true,
		Min:     getCompileContents(min, true),
		Max:     getCompileContents(max, false),
		Ip:      req.Header.Get("X-Forwarded-For"),
	}

	if err := store.Save(ctx, path, data); err != nil {
		// don't save this one to the datastore because it's an error from the datastore.
		sendAndStoreError(ctx, send, path, err, req)
		return
	}

}

func sendAndStoreError(ctx context.Context, send chan messages.Message, path string, err error, req *http.Request) {
	storeError(ctx, path, err, req)
	sendError(send, path, err)
}

func sendError(send chan messages.Message, path string, err error) {
	send <- messages.Error{
		Path:    path,
		Message: err.Error(),
	}
}

func storeError(ctx context.Context, path string, err error, req *http.Request) {

	fmt.Println("error:", err.Error())

	if err == queue.TooManyItemsQueued {
		// If the server is getting flooded by a DOS, this will prevent database flooding
		return
	}

	// ignore errors when logging an error
	store.Save(ctx, path, store.CompileData{
		Path:    path,
		Time:    time.Now(),
		Success: false,
		Error:   err.Error(),
		Ip:      req.Header.Get("X-Forwarded-For"),
	})

}

func (h *Handler) IconHandler(w http.ResponseWriter, req *http.Request) {
	if err := ServeStatic(req.URL.Path, w, req, "image/x-icon"); err != nil {
		http.Error(w, "error serving static file", 500)
	}
}

func (h *Handler) CssHandler(w http.ResponseWriter, req *http.Request) {
	if err := ServeStatic(req.URL.Path, w, req, "text/css"); err != nil {
		http.Error(w, "error serving static file", 500)
	}
}

func (h *Handler) HealthCheckHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(w, "ok")
}

func (h *Handler) GoCheckHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(w, runtime.NumGoroutine())
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mux.ServeHTTP(w, r)
}

func normalizePath(path string) string {

	// We should normalize gist urls by removing the username part
	if strings.HasPrefix(path, "gist.github.com/") {
		matches := gistWithUsername.FindStringSubmatch(path)
		if len(matches) > 1 {
			return fmt.Sprintf("gist.github.com/%s", matches[1])
		}
	}

	// Add github.com if the first part of the path is not a hostname and matches the github username regex
	if strings.Contains(path, "/") {
		firstPart := path[:strings.Index(path, "/")]
		if !strings.Contains(firstPart, ".") && githubUsername.MatchString(firstPart) {
			return fmt.Sprintf("github.com/%s", path)
		}
	}

	return path
}

var gistWithUsername = regexp.MustCompile(`^gist\.github\.com/[A-Za-z0-9_.\-]+/([a-f0-9]+)(/[\p{L}0-9_.\-]+)*$`)
var githubUsername = regexp.MustCompile(`^[a-zA-Z0-9\-]{0,38}$`)

func ServeStatic(name string, w http.ResponseWriter, req *http.Request, mimeType string) error {
	var file billy.File
	var err error
	file, err = assets.Assets.Open(name)
	if err != nil {
		if os.IsNotExist(err) {
			http.NotFound(w, req)
			return nil
		}
		http.Error(w, fmt.Sprintf("error opening %s", name), 500)
		return nil
	}
	defer file.Close()

	w.Header().Set("Cache-Control", "max-age=31536000")
	if mimeType == "" {
		w.Header().Set("Content-Type", mime.TypeByExtension(pathpkg.Ext(req.URL.Path)))
	} else {
		w.Header().Set("Content-Type", mimeType)
	}

	_, noCompress := file.(httpgzip.NotWorthGzipCompressing)
	gzb, isGzb := file.(httpgzip.GzipByter)

	if isGzb && !noCompress && strings.Contains(req.Header.Get("Accept-Encoding"), "gzip") {
		w.Header().Set("Content-Encoding", "gzip")
		if err := WriteWithTimeout(w, gzb.GzipBytes()); err != nil {
			http.Error(w, fmt.Sprintf("error streaming gzipped %s", name), 500)
			return err
		}
	} else {
		if err := StreamWithTimeout(w, file); err != nil {
			http.Error(w, fmt.Sprintf("error streaming %s", name), 500)
			return err
		}
	}
	return nil

}

func StreamWithTimeout(w io.Writer, r io.Reader) error {
	c := make(chan error, 1)
	go func() {
		_, err := io.Copy(w, r)
		c <- err
	}()
	select {
	case err := <-c:
		if err != nil {
			return err
		}
		return nil
	case <-time.After(config.WriteTimeout):
		return errors.New("timeout")
	}
}

func WriteWithTimeout(w io.Writer, b []byte) error {
	return StreamWithTimeout(w, bytes.NewBuffer(b))
}

/*
Sniperkit-Bot
- Status: analyzed
*/

package play

import (
	"context"
	"fmt"
	"html/template"
	"net/http"
	"runtime"
	"strings"

	"github.com/dave/services"

	"github.com/sniperkit/snk.fork.dave-jsgo/config"
	"github.com/sniperkit/snk.fork.dave-jsgo/server/store"
)

func Page(w http.ResponseWriter, req *http.Request, database services.Database) {

	ctx, cancel := context.WithTimeout(req.Context(), config.PageTimeout)
	defer cancel()

	var url string
	if config.DEV {
		url = "/_script.js"
	} else {
		found, c, err := store.Package(ctx, database, "github.com/dave/play")
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		if !found {
			http.Error(w, "play package not found", 500)
			return
		}
		url = fmt.Sprintf("https://pkg.jsgo.io/github.com/dave/play.%s.js", c.Min.Main)
	}

	v := struct {
		Script string
		Count  int
		Prod   bool
	}{
		Script: url,
		Count:  runtime.NumGoroutine(),
		Prod:   !config.DEV,
	}

	if err := playPageTemplate.Execute(w, v); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
}

func asset(url string) string {
	if config.LOCAL {
		return "/_local" + url[strings.LastIndex(url, "/"):]
	}
	return url
}

var playPageTemplate = template.Must(template.New("main").Funcs(template.FuncMap{"Asset": asset}).Parse(`<html>
	<head>
		<meta charset="utf-8">
		{{ if .Prod -}}
		<script async src="https://www.googletagmanager.com/gtag/js?id=UA-118676357-1"></script>
        <script>
            window.dataLayer = window.dataLayer || [];
            function gtag(){dataLayer.push(arguments);}
            gtag('js', new Date());
            gtag('config', 'UA-118676357-1');
        </script>
		{{- end }}
        <link href="{{ Asset "https://maxcdn.bootstrapcdn.com/bootstrap/4.0.0/css/bootstrap.min.css" }}" rel="stylesheet">
        <script src="{{ Asset "https://code.jquery.com/jquery-3.2.1.slim.min.js" }}"></script>
        <script src="{{ Asset "https://cdnjs.cloudflare.com/ajax/libs/popper.js/1.12.9/umd/popper.min.js" }}"></script>
        <script src="{{ Asset "https://maxcdn.bootstrapcdn.com/bootstrap/4.0.0/js/bootstrap.min.js" }}"></script>
        <script src="{{ Asset "https://cdnjs.cloudflare.com/ajax/libs/ace/1.3.3/ace.js" }}"></script>
		<script src="{{ Asset "https://cdnjs.cloudflare.com/ajax/libs/ace/1.3.3/ext-linking.js" }}"></script>
	</head>
	<body id="wrapper" style="margin: 0;" data-count="{{ .Count }}">
		<div id="progress-holder" style="width: 100%; padding: 25%;">
			<div class="progress">
				<div id="progress-bar" class="progress-bar" role="progressbar" style="width: 0%" aria-valuenow="0" aria-valuemin="0" aria-valuemax="100"></div>
			</div>
		</div>
		<script>
			window.jsgoProgress = function(count, total) {
				var value = (count * 100.0) / (total * 1.0);
				var bar = document.getElementById("progress-bar");
				bar.style.width = value+"%";
				bar.setAttribute('aria-valuenow', value);
				if (count === total) {
					document.getElementById("progress-holder").style.display = "none";
				}
			}
		</script>
    	<script src="{{ .Script }}"></script>
	</body>
</html>`))

package main

import (
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/gomarkdown/markdown"
	"github.com/urfave/cli/v2"
)

type Options struct {
	MarkdownFile string
	HtmlContent  chan []byte
	FileMutext   sync.Mutex
}

func html_page() []byte {
	htmlContent := []byte(`
<!DOCTYPE html>
<html>
<head>
	<title>Markdown Preview</title>
	<link rel="stylesheet" href="https://fonts.googleapis.com/css?family=Roboto:400,400i,700&display=swap">
	<style>
    body {
        font-family: 'Roboto', sans-serif;
    }
	</style>
</head>
<body style="padding: 3px" >
	<div id="sse-data">Preview Server booting up</div>

	<script>
		const event_source = new EventSource('http://localhost:8080/events');
		event_source.onmessage = function(event) {
			const data_element = document.getElementById('sse-data');
			data_element.innerHTML = atob(event.data);
		};
		event_source.onerror = function(event) {
			const data_element = document.getElementById('sse-data');
			data_element.innerHTML = 'Preview server is down';
		}
	</script>
</body>
</html>
`)
	return htmlContent
}

func load_file(o *Options) {
	o.FileMutext.Lock()
	markdown_bytes, err := os.ReadFile(o.MarkdownFile)
	if err != nil {
		log.Println("Error: ", err)
	}
	unsafe_html := markdown.ToHTML(markdown_bytes, nil, nil)
	html_content := Shrink_double_linefeeds(unsafe_html)
	encoded_len := base64.StdEncoding.EncodedLen(len(html_content))
	base64_html_content := make([]byte, encoded_len)
	base64.StdEncoding.Encode(base64_html_content, html_content)
	o.HtmlContent <- base64_html_content
	o.FileMutext.Unlock()
}

func hot_loader(o *Options) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
	}
	defer watcher.Close()

	load_file(o)

	done := make(chan bool)
	defer close(done)

	abs_path, err := filepath.Abs(o.MarkdownFile)
	if err != nil {
		fmt.Println(err)
	}

	// Start a go routine to watch for events
	go func() {
		for {
			select {
			case event := <-watcher.Events:
				abs_name_path, err := filepath.Abs(event.Name)
				if err != nil {
					fmt.Println(err)
				}
				// Check if it's the markdown file that the prog is watching.
				if abs_path == abs_name_path {
					// Only reload if it's the Create or the Write event
					if event.Op == fsnotify.Create || event.Op == fsnotify.Write {
						load_file(o)
					}
				}
			case err := <-watcher.Errors:
				log.Println("Error: ", err)
			}
		}
	}()

	parent := filepath.Dir(o.MarkdownFile)
	err = watcher.Add(parent)
	if err != nil {
		log.Fatal(err)
	}

	<-done
}

func render_markdown(o *Options) error {
	// Serve HTML to browser
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write(html_page())
		time.Sleep(time.Second * 2)
		go load_file(o)
	})
	http.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
		// Set CORS headers to allow all origins - not wise for public servers
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Expose-Headers", "Content-Type")

		// Set headers for SSE
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		// Create a channel to notify when content is updated
		contentUpdate := make(chan []byte)
		defer close(contentUpdate)

		// Goroutine to send updated content to the client
		go func() {
			for {
				content := <-o.HtmlContent
				contentUpdate <- content
			}
		}()

		// Wait for updates and send them to the client
		for {
			content := <-contentUpdate
			fmt.Fprintf(w, "data: %s\n\n", content)
			w.(http.Flusher).Flush()
		}
	})

	// Start the web server
	port := ":8080"
	fmt.Printf("Preview Server running on port %s\n", port)

	return http.ListenAndServe(port, nil)
}

func main() {
	app := &cli.App{
		Name:      "md-preview",
		Usage:     "Preview markdown file in the browser - with hot reloading",
		UsageText: "md-preview <filename>",
		Action: func(c *cli.Context) error {
			filename := c.Args().First()
			if filename == "" {
				log.Fatalf("missing markdown file argument\n")
			}
			options := Options{
				MarkdownFile: c.Args().First(),
				HtmlContent:  make(chan []byte, 1),
			}
			defer close(options.HtmlContent)
			go hot_loader(&options)
			return render_markdown(&options)
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

}

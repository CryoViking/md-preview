package main

import (
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/gomarkdown/markdown"
	"github.com/urfave/cli/v2"
)

type Options struct {
	MarkdownFile  string
	HtmlContent   chan []byte
	FileMutext    sync.Mutex
	ServerPort    uint16
	ServerAddress string
}

func html_page(o *Options) []byte {
	// htmlContent := []byte(`
	htmlContent := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
	<title>Markdown Preview</title>
	<link rel="stylesheet" href="https://fonts.googleapis.com/css?family=Roboto:400,400i,700&display=swap">
  <style>
    html {
      line-height: 1.5;
      font-family: Georgia, serif;
      font-size: 20px;
      color: #1a1a1a;
      background-color: #fdfdfd;
    }
    body {
       font-family: 'Roboto', sans-serif;
      margin: 0 auto;
      max-width: 36em;
      padding-left: 50px;
      padding-right: 50px;
      padding-top: 50px;
      padding-bottom: 50px;
      hyphens: auto;
      overflow-wrap: break-word;
      text-rendering: optimizeLegibility;
      font-kerning: normal;
    }
    @media (max-width: 600px) {
      body {
        font-size: 0.9em;
        padding: 1em;
      }
      h1 {
        font-size: 1.8em;
      }
    }
    @media print {
      body {
        background-color: transparent;
        color: black;
        font-size: 12pt;
      }
      p, h2, h3 {
        orphans: 3;
        widows: 3;
      }
      h2, h3, h4 {
        page-break-after: avoid;
      }
    }
    p {
      margin: 1em 0;
    }
    a {
      color: #1a1a1a;
    }
    a:visited {
      color: #1a1a1a;
    }
    img {
      max-width: 100%;
    }
    h1, h2, h3, h4, h5, h6 {
      margin-top: 1.4em;
    }
    h5, h6 {
      font-size: 1em;
      font-style: italic;
    }
    h6 {
      font-weight: normal;
    }
    ol, ul {
      padding-left: 1.7em;
      margin-top: 1em;
    }
    li > ol, li > ul {
      margin-top: 0;
    }
    blockquote {
      margin: 1em 0 1em 1.7em;
      padding-left: 1em;
      border-left: 2px solid #e6e6e6;
      color: #606060;
    }
    code {
      font-family: Menlo, Monaco, 'Lucida Console', Consolas, monospace;
      font-size: 85%;
      margin: 0;
    }
    pre {
      margin: 1em 0;
      overflow: auto;
    }
    pre code {
      padding: 0;
      overflow: visible;
      overflow-wrap: normal;
    }
    .sourceCode {
     background-color: transparent;
     overflow: visible;
    }
    hr {
      background-color: #1a1a1a;
      border: none;
      height: 1px;
      margin: 1em 0;
    }
    table {
      margin: 1em 0;
      border-collapse: collapse;
      width: 100%;
      overflow-x: auto;
      display: block;
      font-variant-numeric: lining-nums tabular-nums;
    }
    table caption {
      margin-bottom: 0.75em;
    }
    tbody {
      margin-top: 0.5em;
      border-top: 1px solid #1a1a1a;
      border-bottom: 1px solid #1a1a1a;
    }
    th {
      border-top: 1px solid #1a1a1a;
      padding: 0.25em 0.5em 0.25em 0.5em;
    }
    td {
      padding: 0.125em 0.5em 0.25em 0.5em;
    }
    header {
      margin-bottom: 4em;
      text-align: center;
    }
    #TOC li {
      list-style: none;
    }
    #TOC ul {
      padding-left: 1.3em;
    }
    #TOC > ul {
      padding-left: 0;
    }
    #TOC a:not(:hover) {
      text-decoration: none;
    }
    code{white-space: pre-wrap;}
    span.smallcaps{font-variant: small-caps;}
    div.columns{display: flex; gap: min(4vw, 1.5em);}
    div.column{flex: auto; overflow-x: auto;}
    div.hanging-indent{margin-left: 1.5em; text-indent: -1.5em;}
    ul.task-list{list-style: none;}
    ul.task-list li input[type="checkbox"] {
      width: 0.8em;
      margin: 0 0.8em 0.2em -1.6em;
      vertical-align: middle;
    }
    .display.math{display: block; text-align: center; margin: 0.5rem auto;}
	</style>
</head>
<body style="padding: 3px" >
	<div id="sse-data">Preview Server booting up</div>

	<script>
` + fmt.Sprintf(`
		const event_source = new EventSource('http://%s:%v/events');
    `, o.ServerAddress, o.ServerPort) + fmt.Sprintf(` 
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
`))
	return []byte(htmlContent)
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
		w.Write(html_page(o))
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
	fmt.Printf("Preview Server running at http://%s:%v\n", o.ServerAddress, o.ServerPort)

	return http.ListenAndServe(fmt.Sprintf("%s:%v", o.ServerAddress, o.ServerPort), nil)
}

func valid_ip(ip string) bool {
	if ip == "localhost" || ip == "any" {
		return true
	}
	r, _ := regexp.Compile(`^(25[0-5]|2[0-4][0-9]|[0-1]{1}[0-9]{2}|[1-9]{1}[0-9]{1}|[1-9])\.(25[0-5]|2[0-4][0-9]|[0-1]{1}[0-9]{2}|[1-9]{1}[0-9]{1}|[1-9]|0)\.(25[0-5]|2[0-4][0-9]|[0-1]{1}[0-9]{2}|[1-9]{1}[0-9]{1}|[1-9]|0)\.(25[0-5]|2[0-4][0-9]|[0-1]{1}[0-9]{2}|[1-9]{1}[0-9]{1}|[0-9])$`)
	return r.MatchString(ip)
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
			if c.Int("port") < 1 || c.Int("port") > 65535 {
				log.Fatalf("invalid port number: %d. Valid range is 1-65535", c.Int("port"))
			}
			var addr string
			if !valid_ip(c.String("address")) {
				log.Fatalf("Invalid address: %s. Please give a valid ip, 'localhost' or 'any'", c.String("address"))
			} else {
				if c.String("address") == "any" {
					addr = "0.0.0.0"
				} else {
					addr = c.String("address")
				}
			}

			options := Options{
				MarkdownFile:  c.Args().First(),
				HtmlContent:   make(chan []byte, 1),
				ServerPort:    uint16(c.Int("port")),
				ServerAddress: addr,
			}
			defer close(options.HtmlContent)
			go hot_loader(&options)
			return render_markdown(&options)
		},
		Flags: []cli.Flag{
			&cli.IntFlag{
				Name:  "port",
				Value: 8080,
				Usage: "Sets the port for the server to run on",
			},
			&cli.StringFlag{
				Name:  "address",
				Value: "localhost",
				Usage: "Change the event emitter to listen on another address",
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

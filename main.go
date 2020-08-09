package main

import (
	"fmt"
	"html/template"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"strconv"
	"strings"
	"syscall"
	"time"

	qrcode "github.com/skip2/go-qrcode"
)

func main() {
	listFiles := os.Args[1:]
	for _, v := range listFiles {
		fmt.Printf("%s -> %s\n", v, path.Base(v))
	}

	exit := make(chan os.Signal, 1)
	signal.Notify(exit, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-exit
		os.Exit(0)
	}()
	serve(listFiles)
}

func serve(listFiles []string) {
	var address = ""
	port := 3360
	local := false

	listener, err := listen(address, port)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	if !local {
		// Get the outbound IP
		address = getOutboundIP().String()
	}

	www := fmt.Sprintf("http://%s:%d", address, listener.Addr().(*net.TCPAddr).Port)
	fmt.Printf("Listening on: %s\n", www)

	dlHandler := newDownloadHandler(listFiles)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			dlHandler(w, r)
			return
		}
		renderIndex(w, listFiles)
	})

	png, _ := qrcode.Encode(www, qrcode.Medium, 256)
	http.HandleFunc("/qr", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Header().Set("content-type", "image/png")
		w.Write(png)
	})
	http.HandleFunc("/stop", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Stopped server. Goodbye ;)"))
		go func() {
			// Wait for a second for the response to finish
			time.Sleep(500 * time.Millisecond)
			os.Exit(0)
		}()
	})

	// Open web browser
	go func() {
		<-time.NewTimer(500 * time.Millisecond).C
		exec.Command("open", www).Run()
	}()

	// start server
	if err := http.Serve(listener, nil); err != nil {
		panic(err)
	}
}

func listen(address string, port int) (net.Listener, error) {
	addr := fmt.Sprintf("%s:%d", address, port)

	if listener, err := net.Listen("tcp", addr); err == nil {
		return listener, nil
	}

	fmt.Println("Address in use. Trying to stop the old server ...")
	client := http.Client{Timeout: 20 * time.Second}
	client.Get(fmt.Sprintf("http://localhost:%d/stop", port))

	time.Sleep(1 * time.Second)

	// Try to listen again
	return net.Listen("tcp", addr)
}

var timer *time.Timer

func renderIndex(w http.ResponseWriter, listFiles []string) {
	if len(listFiles) == 0 {
		w.Write([]byte("No file to serve. The server will stop in next 5 seconds"))

		if timer == nil {
			timer = time.NewTimer(5 * time.Second)
			go func() {
				<-timer.C
				os.Exit(0)
			}()
		}
		return
	}

	w.Header().Set("content-type", "text/html")

	fileNames := make([]string, len(listFiles))
	for k, v := range listFiles {
		fileNames[k] = path.Base(v)
	}

	contentTemplate := `
	<!DOCTYPE html>
	<html itemscope itemtype="http://schema.org/QAPage" class="html__responsive">
	<head><meta charset="UTF-8"><meta name="viewport" content="width=device-width, height=device-height, initial-scale=1.0, minimum-scale=1.0">
	<style>.download { padding: 6px 20px;}</style>
	</head>
	<body>
		<div>
		<div>Address:</div>
		<img src="/qr" width="200" />
		<h4>List files:</h4>
		<table>
			{{range $val := .}}
			<tr>
				<td><a href="/{{$val}}">{{$val}}</a><td>
				<td class="download"><a href="/{{$val}}?download">Download</a></td>
			</tr>
			{{end}}
		</table>
		</br>
		</br>
		<a href="/stop">Done</a>
		</div>
	</body>
`

	t := template.Must(template.New("tmpl").Parse(contentTemplate))

	t.Execute(w, fileNames)
}

func newDownloadHandler(listFiles []string) http.HandlerFunc {
	m := make(map[string]string, len(listFiles))

	for _, fn := range listFiles {
		m[path.Base(fn)] = fn
	}
	return func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(r.URL.Path, "/")
		filename := parts[len(parts)-1]
		if filePath, ok := m[filename]; ok {
			if _, forceDownload := r.URL.Query()["download"]; forceDownload {
				w.Header().Set("Content-Disposition", "attachment; filename="+strconv.Quote(filename))
				w.Header().Set("Content-Type", "application/octet-stream")
			}
			http.ServeFile(w, r, filePath)
		} else {
			http.NotFound(w, r)
		}
	}
}

func getOutboundIP() net.IP {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP
}

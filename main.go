package main

import (
	"flag"
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

	"github.com/reeganexe/go-portfwd"
	"github.com/skip2/go-qrcode"
	"github.com/valyala/fasthttp"
)

var (
	port     = 3360
	nodePath = "/usr/local/bin/node"
)

var stopServer func()

func main() {
	var nodePath = "node"
	flag.IntVar(&port, "port", port, "Port")
	flag.StringVar(&nodePath, "node", nodePath, "node path")
	flag.Parse()

	listFiles := flag.Args()

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
	listener, err := tryListen(port)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}

	stopServer = func() {
		listener.Close()
		os.Exit(0)
	}

	// Get the outbound IP
	publicAddr := getOutboundIP().String()

	www := fmt.Sprintf("http://%s:%d", publicAddr, port)
	fmt.Printf("Listening on: %s\n", www)

	dlHandler := newDownloadHandler(listFiles)

	png, _ := qrcode.Encode(www, qrcode.Medium, 256)
	m := func(ctx *fasthttp.RequestCtx) {
		switch string(ctx.Path()) {
		case "/qr":
			ctx.Response.Header.SetContentType("image/png")
			ctx.Write(png)
		case "/stop":
			ctx.Write([]byte("Stopped server. Goodbye ;)"))
			go func() {
				// Wait for a second for the response to finish
				time.AfterFunc(500*time.Millisecond, stopServer)
			}()
		case "/":
			renderIndex(ctx, listFiles)
		default:
			dlHandler(ctx)
		}
	}

	// Open web browser
	go func() {
		<-time.NewTimer(500 * time.Millisecond).C
		exec.Command("open", www).Run()
	}()

	// start server
	if err := fasthttp.Serve(listener.Listener, m); err != nil {
		panic(err)
	}
}

func tryListen(port int) (*portfwd.ForwarderListener, error) {
	if listener, e := portfwd.ListenAndForward(port, nodePath); e == nil {
		return listener, nil
	}

	tryStopPort(port)
	time.Sleep(time.Second)
	return portfwd.ListenAndForward(port, nodePath)
}

func tryStopPort(port int) {
	fmt.Println("Address in use. Trying to stop the old server ...")
	client := http.Client{Timeout: 1 * time.Second}
	client.Get(fmt.Sprintf("http://localhost:%d/stop", port))
}

var timer *time.Timer

func renderIndex(ctx *fasthttp.RequestCtx, listFiles []string) {
	if len(listFiles) == 0 {
		ctx.Write([]byte("No file to serve. The server will stop in next 5 seconds"))

		if timer == nil {
			timer = time.NewTimer(5 * time.Second)
			go func() {
				<-timer.C
				os.Exit(0)
			}()
		}
		return
	}

	ctx.Response.Header.SetContentType("text/html")

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
				<td class="download"><a href="/{{$val}}?download&stop">Download & Stop</a></td>
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

	t.Execute(ctx.Response.BodyWriter(), fileNames)
}

func newDownloadHandler(listFiles []string) fasthttp.RequestHandler {
	m := make(map[string]string, len(listFiles))

	for _, fn := range listFiles {
		m[path.Base(fn)] = fn
	}
	return func(ctx *fasthttp.RequestCtx) {
		parts := strings.Split(string(ctx.Path()), "/")
		filename := parts[len(parts)-1]
		if filePath, ok := m[filename]; ok {
			if ctx.URI().QueryArgs().Has("download") {
				ctx.Response.Header.Set("Content-Disposition", "attachment; filename="+strconv.Quote(filename))
				ctx.Response.Header.SetContentType("application/octet-stream")
			}

			fasthttp.ServeFile(ctx, filePath)
			if ctx.URI().QueryArgs().Has("stop") {
				go func() {
					time.AfterFunc(time.Second, stopServer)
				}()
			}
		} else {
			ctx.NotFound()
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

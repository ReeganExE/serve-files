package main

import (
	"fmt"
	"net"
	"os"
	"os/exec"
)

type ForwarderListener struct {
	Listener net.Listener
	Addr     *net.TCPAddr
	Close    killer
}

type killer func() error

func newForwarderListener(port int, nodePath string) (*ForwarderListener, error) {
	internalAddr := fmt.Sprintf("localhost:%d", 0)
	listener, e := net.Listen("", internalAddr)

	if e != nil {
		return nil, e
	}

	tcpAddr := listener.Addr().(*net.TCPAddr)
	internalAddr = fmt.Sprintf("localhost:%d", tcpAddr.Port)
	publicAddr := fmt.Sprintf("0.0.0.0:%d", port)
	kill, e := forwardPort(nodePath, publicAddr, internalAddr)
	if e != nil {
		return nil, e
	}
	return &ForwarderListener{Listener: listener, Close: kill, Addr: tcpAddr}, nil
}

func forwardPort(nodePath, from, to string) (killer, error) {
	cmd := exec.Command(nodePath, "-e", proxyjs, "..", from, to)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Process.Kill, cmd.Start()
}

var proxyjs = `
var net = require('net');

var addrRegex = /^(([a-zA-Z\-\.0-9]+):)?(\d+)$/;

var addr = {
  from: addrRegex.exec(process.argv[2]),
  to: addrRegex.exec(process.argv[3]),
};

if (!addr.from || !addr.to) {
  console.log('Usage: <from> <to>');
  process.exit(1);
}

const server = net
  .createServer(function (from) {
    var to = net.createConnection({
      host: addr.to[2],
      port: addr.to[3],
    });
    from
      .pipe(to)
      .on('error', (e) => {
        console.warn('Send error', e);
        from.destroy();
      });
    to.pipe(from).on('error', e => {
      console.warn('response error', e);
    });
  })
  .listen(addr.from[3], addr.from[2]);

process.on('SIGTERM', function () {
  server.close();
});
`

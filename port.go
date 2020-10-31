package main

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"os/exec"
	"time"

	"errors"
)

// ForwarderListener contains the internal listener and a dispose function to close the proxy
type ForwarderListener struct {
	// Listener Internal listener
	Listener net.Listener
	// Addr internal listener addr
	Addr  *net.TCPAddr
	Close func() error
}

// ListenAndForward listens on a random port, then starts a node.js process on the specificed port
// The app will have to use ForwarderListener.Listener to serve its content
func ListenAndForward(port int, nodeExecutablePath string) (*ForwarderListener, error) {
	internalAddr := fmt.Sprintf("localhost:%d", 0)
	internalListener, e := net.Listen("tcp", internalAddr)

	if e != nil {
		return nil, e
	}

	internalTCPAddr := internalListener.Addr().(*net.TCPAddr)
	internalAddr = fmt.Sprintf("localhost:%d", internalTCPAddr.Port)
	publicAddr := fmt.Sprintf("0.0.0.0:%d", port)

	// Due to handle error from child process makes this complicated
	// We will go with the solution to check the port before listening on it
	timeout := time.Second
	conn, _ := net.DialTimeout("tcp", publicAddr, timeout)
	if conn != nil {
		conn.Close()
		return nil, errors.New("port in use")
	}

	cmd := forwardPort(nodeExecutablePath, publicAddr, internalAddr)
	e = cmd.Start()
	if e != nil {
		return nil, e
	}

	return &ForwarderListener{Listener: internalListener, Close: cmd.Process.Kill, Addr: internalTCPAddr}, nil
}

func forwardPort(nodePath, from, to string) *exec.Cmd {
	cmd := exec.Command(nodePath, "-", from, to)
	buffer := &bytes.Buffer{}
	buffer.WriteString(ProxyJS)
	cmd.Stdin = buffer
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd
}

// ProxyJS A JavaScript code to run the forwarder
var ProxyJS = `
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

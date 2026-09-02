package websocket_test

import (
	"context"
	"encoding/base64"
	"net/http"
	"testing"
	"time"

	gorillawebsocket "github.com/gorilla/websocket"
	"github.com/xtls/xray-core/common"
	"github.com/xtls/xray-core/common/net"
	"github.com/xtls/xray-core/testing/servers/tcp"
	"github.com/xtls/xray-core/transport/internet"
	"github.com/xtls/xray-core/transport/internet/stat"
	. "github.com/xtls/xray-core/transport/internet/websocket"
)

func TestDialEarlyDataUsesReferer(t *testing.T) {
	listenPort := tcp.PickPort()
	earlyData := []byte("early data over referer")
	readCh := make(chan []byte, 1)
	errCh := make(chan error, 1)

	listen, err := ListenWS(context.Background(), net.LocalHostIP, listenPort, &internet.MemoryStreamConfig{
		ProtocolName:     "websocket",
		ProtocolSettings: &Config{Path: "ws"},
	}, func(conn stat.Connection) {
		go func() {
			defer conn.Close()
			var b [1024]byte
			conn.SetReadDeadline(time.Now().Add(2 * time.Second))
			n, err := conn.Read(b[:])
			if err != nil {
				errCh <- err
				return
			}
			readCh <- append([]byte(nil), b[:n]...)
		}()
	})
	common.Must(err)
	defer listen.Close()

	conn, err := Dial(context.Background(), net.TCPDestination(net.DomainAddress("localhost"), listenPort), &internet.MemoryStreamConfig{
		ProtocolName:     "websocket",
		ProtocolSettings: &Config{Path: "ws", Ed: uint32(len(earlyData))},
	})
	common.Must(err)
	defer conn.Close()

	n, err := conn.Write(earlyData)
	common.Must(err)
	if n != len(earlyData) {
		t.Fatalf("unexpected write length: got %d want %d", n, len(earlyData))
	}

	select {
	case got := <-readCh:
		if string(got) != string(earlyData) {
			t.Fatalf("unexpected early data: got %q want %q", got, earlyData)
		}
	case err := <-errCh:
		t.Fatal(err)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for early data")
	}
}

func TestServerAcceptsSubprotocolEarlyData(t *testing.T) {
	listenPort := tcp.PickPort()
	earlyData := []byte("early data over websocket protocol")
	readCh := make(chan []byte, 1)
	errCh := make(chan error, 1)

	listen, err := ListenWS(context.Background(), net.LocalHostIP, listenPort, &internet.MemoryStreamConfig{
		ProtocolName:     "websocket",
		ProtocolSettings: &Config{Path: "ws"},
	}, func(conn stat.Connection) {
		go func() {
			defer conn.Close()
			var b [1024]byte
			conn.SetReadDeadline(time.Now().Add(2 * time.Second))
			n, err := conn.Read(b[:])
			if err != nil {
				errCh <- err
				return
			}
			readCh <- append([]byte(nil), b[:n]...)
		}()
	})
	common.Must(err)
	defer listen.Close()

	protocol := base64.RawURLEncoding.EncodeToString(earlyData)
	header := http.Header{}
	header.Set("Sec-WebSocket-Protocol", protocol)
	conn, _, err := gorillawebsocket.DefaultDialer.Dial("ws://localhost:"+listenPort.String()+"/ws", header)
	common.Must(err)
	defer conn.Close()
	if got := conn.Subprotocol(); got != protocol {
		t.Fatalf("unexpected subprotocol: got %q want %q", got, protocol)
	}

	select {
	case got := <-readCh:
		if string(got) != string(earlyData) {
			t.Fatalf("unexpected early data: got %q want %q", got, earlyData)
		}
	case err := <-errCh:
		t.Fatal(err)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for early data")
	}
}
